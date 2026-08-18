package aws

import (
	"context"
	"strings"
	"testing"

	types2 "github.com/aws/aws-sdk-go-v2/service/bedrock/types"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime/types"
)

// Arbitrarily split model deltas must reassemble into stable transcript lines.
func TestLineBufferReassemblesDeltas(t *testing.T) {
	tests := []struct {
		name   string
		deltas []string
		want   []string
	}{
		{
			name:   "one line in pieces",
			deltas: []string{"aws ", "s3 ", "ls\n"},
			want:   []string{"aws s3 ls"},
		},
		{
			name:   "several lines in one delta",
			deltas: []string{"first\nsecond\nthird\n"},
			want:   []string{"first", "second", "third"},
		},
		{
			name:   "a delta that straddles a newline",
			deltas: []string{"end of one\nstart of", " two\n"},
			want:   []string{"end of one", "start of two"},
		},
		{
			name:   "blank lines are lines too, markdown needs them",
			deltas: []string{"a paragraph\n", "\n", "another\n"},
			want:   []string{"a paragraph", "", "another"},
		},
		{
			name:   "a trailing line with no newline still arrives",
			deltas: []string{"no trailing newline"},
			want:   []string{"no trailing newline"},
		},
		{
			name:   "nothing in, nothing out",
			deltas: nil,
			want:   nil,
		},
		{
			name:   "a newline on its own",
			deltas: []string{"\n"},
			want:   []string{""},
		},
	}

	for _, tt := range tests {
		var got []string
		buffer := &lineBuffer{onLine: func(line string) { got = append(got, line) }}
		for _, delta := range tt.deltas {
			buffer.write(delta)
		}
		buffer.flush()

		if strings.Join(got, "|") != strings.Join(tt.want, "|") {
			t.Errorf("%s: lines = %q, want %q", tt.name, got, tt.want)
		}
	}
}

func TestLineBufferFlushIsIdempotent(t *testing.T) {
	var got []string
	buffer := &lineBuffer{onLine: func(line string) { got = append(got, line) }}

	buffer.write("done\n")
	buffer.flush()
	buffer.flush()

	if len(got) != 1 || got[0] != "done" {
		t.Errorf("lines = %q, want exactly one", got)
	}
}

// Chat entry points must fail safely before a profile has connected.
func TestStreamChatRejectsUnusableRequests(t *testing.T) {
	ask := []ChatMessage{{FromUser: true, Text: "hi"}}

	tests := []struct {
		name   string
		client *Client
		req    ChatRequest
	}{
		{"no client at all", nil, ChatRequest{Model: "m", Messages: ask}},
		{"client with no bedrock session", &Client{}, ChatRequest{Model: "m", Messages: ask}},
		{"empty prompt", &Client{}, ChatRequest{Model: "m", Messages: []ChatMessage{{FromUser: true, Text: "   "}}}},
		{"no messages", &Client{}, ChatRequest{Model: "m"}},
		{"no model", &Client{}, ChatRequest{Messages: ask}},
	}

	for _, tt := range tests {
		called := false
		err := tt.client.StreamChat(context.Background(), tt.req, func(string) { called = true })
		if err == nil {
			t.Errorf("%s: StreamChat() error = nil, want a refusal", tt.name)
		}
		if called {
			t.Errorf("%s: StreamChat() produced output", tt.name)
		}
	}
}

func TestListChatModelsWithoutASession(t *testing.T) {
	if _, err := (*Client)(nil).ListChatModels(context.Background()); err == nil {
		t.Error("ListChatModels() error = nil, want a refusal with no session")
	}
	if _, err := (&Client{}).ListChatModels(context.Background()); err == nil {
		t.Error("ListChatModels() error = nil, want a refusal with no bedrock session")
	}
}

// Region-specific inference profiles must resolve from one configured model ID.
func TestMatchInferenceProfile(t *testing.T) {
	euSonnet := inferenceProfile{
		ID:       "eu.anthropic.claude-sonnet-4-6-v1:0",
		ModelIDs: []string{"anthropic.claude-sonnet-4-6-v1:0"},
	}
	usSonnet := inferenceProfile{
		ID:       "us.anthropic.claude-sonnet-4-6-v1:0",
		ModelIDs: []string{"anthropic.claude-sonnet-4-6-v1:0"},
	}
	euHaiku := inferenceProfile{
		ID:       "eu.anthropic.claude-haiku-4-5-20251001-v1:0",
		ModelIDs: []string{"anthropic.claude-haiku-4-5-20251001-v1:0"},
	}
	// Profile IDs remain the fallback when AWS omits model metadata.
	apacSonnet := inferenceProfile{ID: "apac.anthropic.claude-sonnet-4-6-v1:0"}

	tests := []struct {
		name     string
		profiles []inferenceProfile
		want     string
		region   string
		expected string
	}{
		{
			name:     "unversioned model id resolves to the region's profile",
			profiles: []inferenceProfile{euHaiku, usSonnet, euSonnet},
			want:     "anthropic.claude-sonnet-4-6",
			region:   "eu-west-1",
			expected: "eu.anthropic.claude-sonnet-4-6-v1:0",
		},
		{
			name:     "the same config resolves to the US profile from a US region",
			profiles: []inferenceProfile{euSonnet, usSonnet},
			want:     "anthropic.claude-sonnet-4-6",
			region:   "us-east-1",
			expected: "us.anthropic.claude-sonnet-4-6-v1:0",
		},
		{
			name:     "no profile for our geo falls back to one that fronts the model",
			profiles: []inferenceProfile{usSonnet},
			want:     "anthropic.claude-sonnet-4-6",
			region:   "eu-west-1",
			expected: "us.anthropic.claude-sonnet-4-6-v1:0",
		},
		{
			name:     "a profile id needs no resolving",
			profiles: []inferenceProfile{euSonnet, usSonnet},
			want:     "eu.anthropic.claude-sonnet-4-6-v1:0",
			region:   "eu-west-1",
			expected: "eu.anthropic.claude-sonnet-4-6-v1:0",
		},
		{
			name:     "a profile with no model list still matches on its id",
			profiles: []inferenceProfile{apacSonnet},
			want:     "anthropic.claude-sonnet-4-6",
			region:   "ap-southeast-2",
			expected: "apac.anthropic.claude-sonnet-4-6-v1:0",
		},
		{
			name:     "nothing fronts it",
			profiles: []inferenceProfile{euHaiku},
			want:     "anthropic.claude-opus-5",
			region:   "eu-west-1",
			expected: "",
		},
		{
			name:     "no profiles at all",
			profiles: nil,
			want:     "anthropic.claude-sonnet-4-6",
			region:   "eu-west-1",
			expected: "",
		},
		{
			name:     "empty want",
			profiles: []inferenceProfile{euSonnet},
			want:     "   ",
			region:   "eu-west-1",
			expected: "",
		},
	}

	for _, tt := range tests {
		if got := matchInferenceProfile(tt.profiles, tt.want, tt.region); got != tt.expected {
			t.Errorf("%s: matchInferenceProfile() = %q, want %q", tt.name, got, tt.expected)
		}
	}
}

func TestModelIDFromARN(t *testing.T) {
	tests := []struct{ arn, want string }{
		{"arn:aws:bedrock:eu-west-1::foundation-model/anthropic.claude-sonnet-4-6-v1:0", "anthropic.claude-sonnet-4-6-v1:0"},
		{"anthropic.claude-sonnet-4-6-v1:0", "anthropic.claude-sonnet-4-6-v1:0"},
		{"", ""},
	}

	for _, tt := range tests {
		if got := modelIDFromARN(tt.arn); got != tt.want {
			t.Errorf("modelIDFromARN(%q) = %q, want %q", tt.arn, got, tt.want)
		}
	}
}

func TestDedupeModels(t *testing.T) {
	models := dedupeModels([]ChatModel{
		{ID: "eu.anthropic.claude-sonnet-4-6-v1:0", Provider: "inference profile"},
		{ID: "amazon.nova-micro-v1:0", Provider: "Amazon"},
		{ID: "eu.anthropic.claude-sonnet-4-6-v1:0", Provider: "Anthropic"},
	})

	if len(models) != 2 {
		t.Fatalf("models = %d, want 2 after dedupe: %+v", len(models), models)
	}
	if models[0].Provider != "inference profile" {
		t.Errorf("kept the second sighting %q, want the first", models[0].Provider)
	}
}

// Profiles fronting non-chat models must not reach the model picker.
func TestProfileFrontsAnyOf(t *testing.T) {
	chatCapable := []string{"anthropic.claude-sonnet-4-6", "amazon.nova-micro-v1:0"}

	tests := []struct {
		name    string
		profile inferenceProfile
		want    bool
	}{
		{
			name:    "fronts a chat model, versioned where the list was not",
			profile: inferenceProfile{ID: "eu.anthropic.claude-sonnet-4-6-v1:0", ModelIDs: []string{"anthropic.claude-sonnet-4-6-v1:0"}},
			want:    true,
		},
		{
			name:    "fronts a chat model, unversioned where the list was versioned",
			profile: inferenceProfile{ID: "eu.amazon.nova-micro", ModelIDs: []string{"amazon.nova-micro"}},
			want:    true,
		},
		{
			name:    "an embedding model is not something to chat with",
			profile: inferenceProfile{ID: "eu.cohere.embed-v4:0", ModelIDs: []string{"cohere.embed-v4:0"}},
			want:    false,
		},
		{
			name:    "nor is a video model",
			profile: inferenceProfile{ID: "eu.twelvelabs.marengo-embed-3-0-v1:0", ModelIDs: []string{"twelvelabs.marengo-embed-3-0-v1:0"}},
			want:    false,
		},
		{
			name:    "a profile with no model list, matched on its id",
			profile: inferenceProfile{ID: "global.anthropic.claude-sonnet-4-6"},
			want:    true,
		},
		{
			name:    "nothing to match against",
			profile: inferenceProfile{ID: "eu.something.else", ModelIDs: []string{"something.else"}},
			want:    false,
		},
	}

	for _, tt := range tests {
		if got := profileFrontsAnyOf(tt.profile, chatCapable); got != tt.want {
			t.Errorf("%s: profileFrontsAnyOf() = %v, want %v", tt.name, got, tt.want)
		}
	}
}

func TestConverseMessages(t *testing.T) {
	messages, err := converseMessages([]ChatMessage{
		{FromUser: true, Text: "which ec2 are running?"},
		{Text: "use aws ec2 describe-instances"},
		{FromUser: true, Text: "and in the other region?"},
	})
	if err != nil {
		t.Fatalf("converseMessages() error = %v", err)
	}

	if len(messages) != 3 {
		t.Fatalf("messages = %d, want 3", len(messages))
	}
	if messages[0].Role != types.ConversationRoleUser || messages[1].Role != types.ConversationRoleAssistant {
		t.Errorf("roles = %v, %v; want user then assistant", messages[0].Role, messages[1].Role)
	}
	if text := messages[2].Content[0].(*types.ContentBlockMemberText).Value; text != "and in the other region?" {
		t.Errorf("last message = %q, want the question just asked", text)
	}
}

// Failed turns must not become malformed Bedrock messages.
func TestConverseMessagesSkipsEmptyTurns(t *testing.T) {
	messages, err := converseMessages([]ChatMessage{
		{FromUser: true, Text: "first"},
		{Text: "   "},
		{FromUser: true, Text: "second"},
	})
	if err != nil {
		t.Fatalf("converseMessages() error = %v", err)
	}
	if len(messages) != 2 {
		t.Errorf("messages = %d, want the blank answer dropped", len(messages))
	}
}

func TestConverseMessagesRejectsWhatTheAPIWould(t *testing.T) {
	if _, err := converseMessages(nil); err == nil {
		t.Error("converseMessages(nil) error = nil, want a refusal")
	}
	if _, err := converseMessages([]ChatMessage{{Text: "an answer with no question"}}); err == nil {
		t.Error("converseMessages() error = nil for a conversation ending on an answer, want a refusal")
	}
}

// Profile-only text models must remain chat-capable even without ON_DEMAND support.
func TestProfileOnlyModelsStillCountAsChatCapable(t *testing.T) {
	profileOnly := []string{"anthropic.claude-sonnet-4-6"}

	sonnetProfile := inferenceProfile{
		ID:       "eu.anthropic.claude-sonnet-4-6-v1:0",
		ModelIDs: []string{"anthropic.claude-sonnet-4-6-v1:0"},
	}
	embedProfile := inferenceProfile{
		ID:       "eu.cohere.embed-v4:0",
		ModelIDs: []string{"cohere.embed-v4:0"},
	}

	if !profileFrontsAnyOf(sonnetProfile, profileOnly) {
		t.Error("a Claude profile was dropped though its model is a text model; that is the bug that made the Claude models vanish from the picker")
	}
	if profileFrontsAnyOf(embedProfile, profileOnly) {
		t.Error("an embedding profile was kept, which is what the filter exists to prevent")
	}
}

func TestHasInferenceType(t *testing.T) {
	onDemand := []types2.InferenceType{types2.InferenceTypeOnDemand}
	provisioned := []types2.InferenceType{types2.InferenceTypeProvisioned}

	if !hasInferenceType(onDemand, types2.InferenceTypeOnDemand) {
		t.Error("hasInferenceType() = false for a model that supports it")
	}
	if hasInferenceType(provisioned, types2.InferenceTypeOnDemand) {
		t.Error("hasInferenceType() = true for a model that does not support it")
	}
	if hasInferenceType(nil, types2.InferenceTypeOnDemand) {
		t.Error("hasInferenceType(nil) = true")
	}
}
