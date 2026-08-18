package aws

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/bedrock"
	bedrocktypes "github.com/aws/aws-sdk-go-v2/service/bedrock/types"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime/types"
)

// chatMaxTokens bounds both terminal output and per-question cost.
const chatMaxTokens = 2048

type ChatRequest struct {
	Model    string
	System   string
	Messages []ChatMessage
}

type ChatMessage struct {
	FromUser bool
	Text     string
}

type ChatModel struct {
	ID       string
	Name     string
	Provider string
}

// StreamChat buffers arbitrary token deltas so callbacks receive whole lines.
func (c *Client) StreamChat(ctx context.Context, req ChatRequest, onLine func(string)) error {
	if c == nil || c.BedrockRuntime == nil {
		return errors.New("no AWS session for the chat")
	}
	messages, err := converseMessages(req.Messages)
	if err != nil {
		return err
	}
	if strings.TrimSpace(req.Model) == "" {
		return errors.New("no model configured")
	}

	modelID := c.resolveChatModel(ctx, req.Model)

	input := &bedrockruntime.ConverseStreamInput{
		ModelId:         aws.String(modelID),
		Messages:        messages,
		InferenceConfig: &types.InferenceConfiguration{MaxTokens: aws.Int32(chatMaxTokens)},
	}
	if system := strings.TrimSpace(req.System); system != "" {
		input.System = []types.SystemContentBlock{&types.SystemContentBlockMemberText{Value: system}}
	}

	out, err := c.BedrockRuntime.ConverseStream(ctx, input)
	if err != nil {
		return fmt.Errorf("bedrock (%s): %w", modelID, err)
	}

	stream := out.GetStream()
	defer stream.Close()

	lines := &lineBuffer{onLine: onLine}
	for event := range stream.Events() {
		delta, ok := event.(*types.ConverseStreamOutputMemberContentBlockDelta)
		if !ok {
			continue
		}
		if text, ok := delta.Value.Delta.(*types.ContentBlockDeltaMemberText); ok {
			lines.write(text.Value)
		}
	}
	lines.flush()

	if err := stream.Err(); err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return ctxErr
		}
		return fmt.Errorf("bedrock (%s): %w", modelID, err)
	}

	return nil
}

// converseMessages omits empty turns because Converse rejects empty content blocks and requires a final user turn.
func converseMessages(conversation []ChatMessage) ([]types.Message, error) {
	messages := make([]types.Message, 0, len(conversation))
	for _, message := range conversation {
		if strings.TrimSpace(message.Text) == "" {
			continue
		}

		role := types.ConversationRoleAssistant
		if message.FromUser {
			role = types.ConversationRoleUser
		}
		messages = append(messages, types.Message{
			Role:    role,
			Content: []types.ContentBlock{&types.ContentBlockMemberText{Value: message.Text}},
		})
	}

	if len(messages) == 0 {
		return nil, errors.New("empty prompt")
	}
	if messages[len(messages)-1].Role != types.ConversationRoleUser {
		return nil, errors.New("a conversation has to end with a question")
	}

	return messages, nil
}

// resolveChatModel passes unknown IDs through because they may be directly invocable and Bedrock can diagnose them authoritatively.
func (c *Client) resolveChatModel(ctx context.Context, model string) string {
	c.chatModelsMu.Lock()
	if resolved, ok := c.chatModelIDs[model]; ok {
		c.chatModelsMu.Unlock()
		return resolved
	}
	c.chatModelsMu.Unlock()

	resolved := model
	if profiles, err := c.listInferenceProfiles(ctx); err == nil {
		if match := matchInferenceProfile(profiles, model, c.Region); match != "" {
			resolved = match
		}
	}

	c.chatModelsMu.Lock()
	if c.chatModelIDs == nil {
		c.chatModelIDs = map[string]string{}
	}
	c.chatModelIDs[model] = resolved
	c.chatModelsMu.Unlock()

	return resolved
}

type inferenceProfile struct {
	ID       string
	Name     string
	ModelIDs []string
}

func (c *Client) listInferenceProfiles(ctx context.Context) ([]inferenceProfile, error) {
	if c == nil || c.Bedrock == nil {
		return nil, errors.New("no AWS session for the chat")
	}

	out, err := c.Bedrock.ListInferenceProfiles(ctx, &bedrock.ListInferenceProfilesInput{
		TypeEquals: bedrocktypes.InferenceProfileTypeSystemDefined,
	})
	if err != nil {
		return nil, err
	}

	profiles := make([]inferenceProfile, 0, len(out.InferenceProfileSummaries))
	for _, summary := range out.InferenceProfileSummaries {
		if summary.InferenceProfileId == nil || summary.Status != bedrocktypes.InferenceProfileStatusActive {
			continue
		}

		profile := inferenceProfile{ID: *summary.InferenceProfileId, Name: stringValue(summary.InferenceProfileName)}
		for _, model := range summary.Models {
			if model.ModelArn == nil {
				continue
			}
			profile.ModelIDs = append(profile.ModelIDs, modelIDFromARN(*model.ModelArn))
		}
		profiles = append(profiles, profile)
	}

	return profiles, nil
}

// matchInferenceProfile prefers the caller's geographic prefix when several profiles front one model.
func matchInferenceProfile(profiles []inferenceProfile, want, region string) string {
	want = strings.TrimSpace(want)
	if want == "" {
		return ""
	}

	geo := ""
	if idx := strings.IndexByte(region, '-'); idx > 0 {
		geo = region[:idx] + "."
	}

	var fallback string
	for _, profile := range profiles {
		if profile.ID == want {
			return want
		}
		if !profileFrontsModel(profile, want) {
			continue
		}
		if geo != "" && strings.HasPrefix(profile.ID, geo) {
			return profile.ID
		}
		if fallback == "" {
			fallback = profile.ID
		}
	}

	return fallback
}

// profileFrontsModel compares prefixes because configured and profile IDs disagree on version suffixes.
func profileFrontsModel(profile inferenceProfile, want string) bool {
	for _, id := range profile.ModelIDs {
		if strings.HasPrefix(id, want) {
			return true
		}
	}

	// Profile ids are the model id with a geo prefix, so they answer the same question when a profile lists no models.
	if idx := strings.IndexByte(profile.ID, '.'); idx > 0 {
		return strings.HasPrefix(profile.ID[idx+1:], want)
	}

	return false
}

// profileFrontsAnyOf compares prefixes in both directions because the two APIs disagree on version suffixes.
func profileFrontsAnyOf(profile inferenceProfile, models []string) bool {
	for _, model := range models {
		if profileFrontsModel(profile, model) {
			return true
		}
		for _, fronted := range profile.ModelIDs {
			if strings.HasPrefix(model, fronted) {
				return true
			}
		}
	}

	return false
}

func modelIDFromARN(arn string) string {
	if idx := strings.LastIndex(arn, "/"); idx >= 0 {
		return arn[idx+1:]
	}

	return arn
}

// ListChatModels returns active streaming text models invocable directly or through a matching inference profile.
func (c *Client) ListChatModels(ctx context.Context) ([]ChatModel, error) {
	if c == nil || c.Bedrock == nil {
		return nil, errors.New("no AWS session for the chat")
	}

	// Do not require ON_DEMAND; profile-only Anthropic models intentionally lack it.
	out, err := c.Bedrock.ListFoundationModels(ctx, &bedrock.ListFoundationModelsInput{
		ByOutputModality: bedrocktypes.ModelModalityText,
	})
	if err != nil {
		return nil, err
	}

	models := make([]ChatModel, 0, len(out.ModelSummaries))
	chatCapable := make([]string, 0, len(out.ModelSummaries))

	for _, summary := range out.ModelSummaries {
		if summary.ModelId == nil {
			continue
		}
		if summary.ResponseStreamingSupported != nil && !*summary.ResponseStreamingSupported {
			continue
		}
		if summary.ModelLifecycle != nil && summary.ModelLifecycle.Status != bedrocktypes.FoundationModelLifecycleStatusActive {
			continue
		}
		// Text in as well as out: an embedding or image model would list TEXT output but can't hold a conversation.
		if !hasModality(summary.InputModalities, bedrocktypes.ModelModalityText) {
			continue
		}

		// Every text model counts for matching profiles; only the ones callable on their own are offered directly.
		chatCapable = append(chatCapable, *summary.ModelId)
		if !hasInferenceType(summary.InferenceTypesSupported, bedrocktypes.InferenceTypeOnDemand) {
			continue
		}

		models = append(models, ChatModel{
			ID:       *summary.ModelId,
			Name:     stringValue(summary.ModelName),
			Provider: stringValue(summary.ProviderName),
		})
	}

	// Bedrock does not label a profile's modality, so only profiles fronting an accepted text model belong in the chat picker.
	if profiles, err := c.listInferenceProfiles(ctx); err == nil {
		for _, profile := range profiles {
			if !profileFrontsAnyOf(profile, chatCapable) {
				continue
			}
			models = append(models, ChatModel{ID: profile.ID, Name: profile.Name, Provider: "inference profile"})
		}
	}

	sort.Slice(models, func(i, j int) bool { return models[i].ID < models[j].ID })

	return dedupeModels(models), nil
}

// dedupeModels prevents a model and its inference profile from appearing twice in the picker.
func dedupeModels(models []ChatModel) []ChatModel {
	seen := make(map[string]bool, len(models))
	kept := make([]ChatModel, 0, len(models))
	for _, model := range models {
		if seen[model.ID] {
			continue
		}
		seen[model.ID] = true
		kept = append(kept, model)
	}

	return kept
}

func hasInferenceType(types []bedrocktypes.InferenceType, want bedrocktypes.InferenceType) bool {
	for _, inferenceType := range types {
		if inferenceType == want {
			return true
		}
	}

	return false
}

func hasModality(modalities []bedrocktypes.ModelModality, want bedrocktypes.ModelModality) bool {
	for _, modality := range modalities {
		if modality == want {
			return true
		}
	}

	return false
}

func stringValue(s *string) string {
	if s == nil {
		return ""
	}

	return *s
}

// lineBuffer reassembles whole lines from the deltas ConverseStream emits, which split wherever the model's tokens happen to land.
type lineBuffer struct {
	pending strings.Builder
	onLine  func(string)
}

func (b *lineBuffer) write(chunk string) {
	for {
		newline := strings.IndexByte(chunk, '\n')
		if newline < 0 {
			b.pending.WriteString(chunk)
			return
		}

		b.pending.WriteString(chunk[:newline])
		b.emit()
		chunk = chunk[newline+1:]
	}
}

// flush preserves the final partial line because models rarely terminate with a newline.
func (b *lineBuffer) flush() {
	if b.pending.Len() > 0 {
		b.emit()
	}
}

func (b *lineBuffer) emit() {
	if b.onLine != nil {
		b.onLine(b.pending.String())
	}
	b.pending.Reset()
}
