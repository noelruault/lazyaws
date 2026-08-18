// Package ui reuses main for chat so scrolling and wrapping stay shared with dashboard details.
package ui

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/jesseduffield/gocui"

	"github.com/noelruault/lazyaws/apps/aws"
	"github.com/noelruault/lazyaws/apps/q"
	"github.com/noelruault/lazyaws/config"
	"github.com/noelruault/lazyaws/ui/types"
)

type qTurn struct {
	question string
	lines    []string
	err      string
	hint     string
	done     bool
}

// qChat retains earlier turns so follow-up questions have context.
type qChat struct {
	started time.Time
	turns   []*qTurn

	// folded belongs to each chat so conversation switches preserve reader choices.
	folded map[int]bool
}

func (c *qChat) title() string {
	if len(c.turns) == 0 {
		return "(empty)"
	}

	return c.turns[0].question
}

func (c *qChat) last() *qTurn {
	if len(c.turns) == 0 {
		return nil
	}

	return c.turns[len(c.turns)-1]
}

// messages excludes failed answers so model context never treats error text as assistant output.
func (c *qChat) messages() []aws.ChatMessage {
	out := make([]aws.ChatMessage, 0, len(c.turns)*2)
	for _, turn := range c.turns {
		out = append(out, aws.ChatMessage{FromUser: true, Text: turn.question})
		if answer := strings.TrimSpace(strings.Join(turn.lines, "\n")); answer != "" {
			out = append(out, aws.ChatMessage{Text: answer})
		}
	}

	return out
}

type qState struct {
	// mu guards transcripts written by task goroutines and read by rendering.
	mu sync.Mutex

	// active stays under mu because background render paths read it.
	active bool

	chats    []*qChat
	selected int

	startNewChat bool

	render qRender

	// width is refreshed by layout because streaming goroutines cannot read view sizes safely.
	width int
}

func (gui *Gui) qScreenActive() bool {
	gui.State.Q.mu.Lock()
	defer gui.State.Q.mu.Unlock()

	return gui.State.Q.active
}

func (gui *Gui) setQScreenActive(active bool) {
	gui.State.Q.mu.Lock()
	defer gui.State.Q.mu.Unlock()

	gui.State.Q.active = active
}

func (gui *Gui) qEnabled() bool {
	return gui.Config != nil && gui.Config.User.Chat.Enabled
}

func (gui *Gui) chatProvider() string {
	if gui.Config != nil && gui.Config.User.Chat.Provider == config.ProviderKiro {
		return config.ProviderKiro
	}

	return config.ProviderBedrock
}

func (gui *Gui) chatModel() string {
	if gui.Config != nil && strings.TrimSpace(gui.Config.User.Chat.Model) != "" {
		return gui.Config.User.Chat.Model
	}

	return config.DefaultChatModel
}

func (gui *Gui) chatBackendLabel() string {
	if gui.chatProvider() == config.ProviderKiro {
		return "Amazon Q (Kiro CLI)"
	}

	return "Bedrock · " + gui.chatModel()
}

func (gui *Gui) chatBackendName() string {
	if gui.chatProvider() == config.ProviderKiro {
		return "Amazon Q"
	}

	return "Bedrock"
}

func (gui *Gui) handleToggleQ() error {
	if gui.qScreenActive() {
		return nil
	}
	// Read-only mode blocks Kiro because it can run tools, while Bedrock can only answer from supplied context.
	if gui.readOnly() && gui.chatProvider() == config.ProviderKiro {
		return gui.refuseReadOnly("The Kiro CLI runs AWS commands with --trust-all-tools, which")
	}
	if !gui.qEnabled() {
		return gui.createConfirmationPanel("Amazon Q is off", qDisabledMessage(), nil, nil)
	}
	if gui.chatProvider() == config.ProviderKiro && !q.Available() {
		return gui.createErrorPanel(q.ErrNotInstalled.Error())
	}

	gui.dismissPopups()
	// Settings wins the shared layout slot, so it must be dismissed before chat becomes active.
	gui.State.Settings.active = false
	gui.setQScreenActive(true)
	gui.Views.Main.Title = gui.chatBackendLabel()
	gui.Views.QInput.Title = "Ask " + gui.chatBackendName()
	gui.Views.Main.Tabs = nil
	gui.Views.Main.TabIndex = 0
	// Double wrapping would break the rendered-row indexes used for code-block clicks.
	gui.Views.Main.Wrap = false
	gui.syncQWidth()

	gui.renderQChats()
	gui.renderQTranscript()
	gui.loadChatModels()

	return gui.switchFocus(gui.Views.QInput)
}

func (gui *Gui) handleQPickModel() error {
	if !gui.qScreenActive() {
		return nil
	}
	if gui.chatProvider() != config.ProviderBedrock {
		return gui.createConfirmationPanel("Chat model", "The Kiro CLI chooses its own model, so there's nothing to pick here.\n\nSwitch the backend to bedrock in Settings (press o) to choose a model from this account.", nil, nil)
	}

	choices := gui.chatModelChoices()
	current := gui.chatModel()

	items := make([]*types.MenuItem, 0, len(choices))
	for _, choice := range choices {
		model := choice
		label := model
		if model == current {
			label = "* " + model
		}
		items = append(items, &types.MenuItem{
			Label:   label,
			OnPress: func() error { return gui.applyChatModel(model) },
		})
	}

	return gui.Menu(CreateMenuOptions{Title: "Chat model", Items: items})
}

// applyChatModel keeps runtime and persisted model selection on one path.
func (gui *Gui) applyChatModel(model string) error {
	gui.Config.User.Chat.Model = model
	gui.refreshChatTitles()

	if err := config.SetStringSetting([]string{"chat", "model"}, model); err != nil {
		return gui.createErrorPanel("Could not save " + config.ConfigFilename() + ":\n\n" + err.Error() + "\n\nThe change applies to this session only.")
	}

	return nil
}

func (gui *Gui) refreshChatTitles() {
	if !gui.qScreenActive() {
		return
	}

	gui.Views.Main.Title = gui.chatBackendLabel()
	gui.Views.QInput.Title = "Ask " + gui.chatBackendName()
}

func (gui *Gui) handleExitQ() error {
	gui.dismissPopups()
	gui.leaveQScreen()

	// Clear synchronously because a queued clear could land after dashboard rendering and erase it.
	gui.Views.Main.Clear()
	_ = gui.Views.Main.SetOrigin(0, 0)
	_ = gui.Views.Main.SetCursor(0, 0)

	// Clearing the transcript's context key forces the dashboard selection to render again.
	gui.State.Panels.Main.ObjectKey = ""

	view, err := gui.g.View(gui.currentSideViewName())
	if err != nil {
		return err
	}

	return gui.switchFocus(view)
}

// leaveQScreen is safe for dashboard jumps to call when chat is already inactive.
func (gui *Gui) leaveQScreen() {
	if !gui.qScreenActive() {
		return
	}

	gui.setQScreenActive(false)
	gui.Views.Main.Title = ""
	gui.Views.Main.Wrap = gui.Config.User.Gui.WrapMainPanel
}

func (gui *Gui) handleQFocusChats() error {
	return gui.switchFocus(gui.Views.QChats)
}

func (gui *Gui) handleQFocusInput() error {
	return gui.switchFocus(gui.Views.QInput)
}

// handleQFocusNext keeps conversation scrolling independent from the input editor.
func (gui *Gui) handleQFocusNext() error {
	if !gui.qScreenActive() {
		return nil
	}

	switch gui.currentViewName() {
	case "qInput":
		return gui.switchFocus(gui.Views.QChats)
	case "qChats":
		return gui.switchFocus(gui.Views.Main)
	default:
		return gui.switchFocus(gui.Views.QInput)
	}
}

func (gui *Gui) handleQMainEscape(g *gocui.Gui, v *gocui.View) error {
	if gui.qScreenActive() {
		return gui.handleExitQ()
	}

	return gui.handleMainEscape(g, v)
}

func (gui *Gui) handleQNewChat() error {
	if !gui.qScreenActive() {
		return nil
	}

	gui.State.Q.mu.Lock()
	gui.State.Q.startNewChat = true
	gui.State.Q.mu.Unlock()

	gui.renderQTranscript()

	return nil
}

func (gui *Gui) handleQSubmit() error {
	question := strings.TrimSpace(gui.Views.QInput.TextArea.GetContent())
	gui.Views.QInput.ClearTextArea()
	gui.Views.QInput.RenderTextArea()

	if question == "" {
		return nil
	}

	return gui.askQ(question)
}

func (gui *Gui) handleQPrevChat() error {
	return gui.selectQChat(gui.State.Q.selected - 1)
}

func (gui *Gui) handleQNextChat() error {
	return gui.selectQChat(gui.State.Q.selected + 1)
}

func (gui *Gui) handleQChatsClick() error {
	_, cy := gui.Views.QChats.Cursor()
	_, oy := gui.Views.QChats.Origin()

	if gui.currentViewName() != "qChats" {
		if err := gui.switchFocus(gui.Views.QChats); err != nil {
			return err
		}
	}

	return gui.selectQChat(cy + oy)
}

func (gui *Gui) selectQChat(idx int) error {
	gui.State.Q.mu.Lock()
	count := len(gui.State.Q.chats)
	if idx < 0 {
		idx = 0
	}
	if idx > count-1 {
		idx = count - 1
	}
	changed := idx >= 0 && idx != gui.State.Q.selected
	if changed {
		gui.State.Q.selected = idx
		// An explicit selection cancels the pending empty conversation.
		gui.State.Q.startNewChat = false
	}
	gui.State.Q.mu.Unlock()

	if !changed {
		return nil
	}

	gui.renderQChats()
	gui.renderQTranscript()

	return nil
}

func qDisabledMessage() string {
	return "The Amazon Q chat is off by default.\n\n" +
		"It answers with your own AWS credentials (Amazon Bedrock by default, the Kiro CLI if you prefer), so lazyaws leaves it off until you ask for it.\n\n" +
		"Turn it on in Settings: press o, put the cursor on \"Amazon Q chat\" and press space. lazyaws writes " + config.ConfigFilename() + " for you."
}

func (gui *Gui) askQ(question string) error {
	// Guard again at the backend boundary so alternate callers cannot bypass feature or read-only policy.
	if !gui.qEnabled() || (gui.readOnly() && gui.chatProvider() == config.ProviderKiro) {
		return nil
	}

	req := q.Request{Prompt: question, Profile: gui.CurrentProfile}
	if gui.Client != nil {
		req.Region = gui.Client.GetRegion()
		req.Context = q.FormatContext(gui.CurrentProfile, gui.Client.GetRegion(), gui.Client.GetAccountID())
	}
	provider, model := gui.chatProvider(), gui.chatModel()
	client := gui.Client

	turn := &qTurn{question: question}

	gui.State.Q.mu.Lock()
	chat := gui.selectedQChatLocked()
	if chat == nil || gui.State.Q.startNewChat {
		chat = &qChat{started: time.Now(), folded: map[int]bool{}}
		gui.State.Q.chats = append([]*qChat{chat}, gui.State.Q.chats...)
		gui.State.Q.selected = 0
		gui.State.Q.startNewChat = false
	}
	chat.turns = append(chat.turns, turn)
	conversation := chat.messages()
	gui.State.Q.mu.Unlock()

	gui.renderQChats()
	gui.renderQTranscript()

	gen := gui.Gen

	// Main-panel task ownership cancels an in-flight query when another render replaces it.
	return gui.QueueTask(gui.NewTask(TaskOpts{
		Autoscroll: true,
		Func: func(ctx context.Context) {
			timeoutCtx, cancel := context.WithTimeout(ctx, q.DefaultTimeout)
			defer cancel()

			gui.streamQAnswer(timeoutCtx, chatBackend{provider: provider, model: model, client: client}, req, conversation, chat, turn, gen)
		},
	}))
}

// chatBackend pins in-flight answers to the backend that started them.
type chatBackend struct {
	provider string
	model    string
	client   *aws.Client
}

func (b chatBackend) stream(ctx context.Context, req q.Request, conversation []aws.ChatMessage, onLine func(string)) error {
	if b.provider == config.ProviderKiro {
		// The CLI is one-shot: it keeps no conversation of its own, so the earlier turns go in as text.
		req.Prompt = flattenConversation(conversation)
		return q.Stream(ctx, req, onLine)
	}

	return b.client.StreamChat(ctx, aws.ChatRequest{
		Model:    b.model,
		System:   chatSystemPrompt + "\n\n" + req.Context,
		Messages: conversation,
	}, onLine)
}

func flattenConversation(conversation []aws.ChatMessage) string {
	if len(conversation) == 1 {
		return conversation[0].Text
	}

	var out strings.Builder
	for _, message := range conversation {
		if message.FromUser {
			out.WriteString("Question: " + message.Text + "\n\n")
			continue
		}
		out.WriteString("Your previous answer: " + message.Text + "\n\n")
	}
	out.WriteString("Answer the last question, taking the conversation above into account.")

	return out.String()
}

func (b chatBackend) retryHint() string {
	if b.provider == config.ProviderKiro {
		return ""
	}

	return "Press ctrl+p to try a different model."
}

// chatSystemPrompt supplies AWS context that Bedrock cannot discover itself.
const chatSystemPrompt = "You are an AWS expert helping someone inspect an AWS account from a terminal UI. " +
	"Answer concisely and concretely. When a task needs a command, give the exact AWS CLI command in a fenced code block. " +
	"You have no access to the account yourself: work from the context you are given, and say plainly when something needs to be checked rather than guessing at values."

// streamQAnswer rejects output invalidated by a profile switch.
func (gui *Gui) streamQAnswer(ctx context.Context, backend chatBackend, req q.Request, conversation []aws.ChatMessage, chat *qChat, turn *qTurn, gen int) {
	update := func(mutate func()) {
		gui.State.Q.mu.Lock()
		mutate()
		gui.State.Q.mu.Unlock()

		if gui.Gen == gen {
			gui.renderQTranscript()
		}
	}

	err := backend.stream(ctx, req, conversation, func(line string) {
		update(func() { turn.lines = append(turn.lines, line) })
	})

	update(func() {
		turn.done = true
		if err != nil {
			turn.err = err.Error()
			turn.hint = backend.retryHint()
		}
		// Merge defaults so earlier turns keep the reader's fold choices.
		for idx, folded := range qFoldDefaults(qChatTranscript(chat)) {
			if _, decided := chat.folded[idx]; !decided {
				chat.folded[idx] = folded
			}
		}
	})

	gui.renderQChats()
}

// renderQTranscript cannot let a streaming query overwrite the dashboard after a screen swap.
func (gui *Gui) renderQTranscript() {
	if !gui.qScreenActive() {
		return
	}

	gui.State.Q.mu.Lock()
	render := renderQMarkdown(gui.qTranscriptLocked(), gui.State.Q.width, gui.foldStateLocked())
	gui.State.Q.render = render
	gui.State.Q.mu.Unlock()

	gui.streamStringMain(render.String())
}

func (gui *Gui) qTranscriptLocked() string {
	chat := gui.selectedQChatLocked()
	if chat == nil || gui.State.Q.startNewChat {
		return "Ask about this account: type a question below and press enter.\n\nThe current profile, region and account go with every question, and the conversation is kept, so you can follow up: \"and in the other region?\" means what it should.\n\nctrl+n starts a fresh conversation, ctrl+p changes model.\n"
	}

	return qChatTranscript(chat)
}

// qChatTranscript requires qState.mu because streaming appends to the last turn.
func qChatTranscript(chat *qChat) string {
	var out strings.Builder

	for i, turn := range chat.turns {
		if i > 0 {
			out.WriteString("\n")
		}
		out.WriteString("> " + turn.question + "\n\n")
		for _, line := range turn.lines {
			out.WriteString(line + "\n")
		}
		if turn.err != "" {
			out.WriteString("\n" + turn.err + "\n")
			if turn.hint != "" {
				out.WriteString("\n" + turn.hint + "\n")
			}
		}
		if !turn.done && len(turn.lines) == 0 {
			out.WriteString("thinking...\n")
		}
	}

	return out.String()
}

func (gui *Gui) foldStateLocked() map[int]bool {
	chat := gui.selectedQChatLocked()
	if chat == nil {
		return map[int]bool{}
	}
	if chat.folded == nil {
		chat.folded = map[int]bool{}
	}

	return chat.folded
}

// syncQWidth runs in layout because reading view dimensions elsewhere races rendering.
func (gui *Gui) syncQWidth() {
	if !gui.qScreenActive() {
		return
	}

	width, _ := gui.Views.Main.Size()

	gui.State.Q.mu.Lock()
	changed := width != gui.State.Q.width
	gui.State.Q.width = width
	gui.State.Q.mu.Unlock()

	if changed {
		gui.renderQTranscript()
	}
}

// handleQConversationClick preserves typing focus and falls through outside chat.
func (gui *Gui) handleQConversationClick() error {
	if !gui.qScreenActive() {
		return gui.handleMainClick()
	}

	_, cy := gui.Views.Main.Cursor()
	_, oy := gui.Views.Main.Origin()

	gui.State.Q.mu.Lock()
	idx := gui.State.Q.render.FoldAt(cy + oy)
	var firstRow int
	var wasFolded bool
	if idx >= 0 {
		firstRow = gui.State.Q.render.Folds[idx].FirstRow
		folds := gui.foldStateLocked()
		wasFolded = folds[idx]
		folds[idx] = !wasFolded
	}
	gui.State.Q.mu.Unlock()

	if idx < 0 {
		return nil
	}

	// Folding disables autoscroll so the next render cannot yank the reader to the bottom.
	gui.Views.Main.Autoscroll = false

	gui.renderQTranscript()

	// Expansion anchors the block header at the top so the reader lands on what they opened.
	if wasFolded {
		_ = gui.Views.Main.SetOrigin(0, firstRow)
	}

	return nil
}

func (gui *Gui) handleQToggleFolds() error {
	if !gui.qScreenActive() {
		return nil
	}

	gui.State.Q.mu.Lock()
	folds := gui.foldStateLocked()
	expand := false
	for _, folded := range folds {
		if folded {
			expand = true
			break
		}
	}
	for idx := range folds {
		folds[idx] = !expand
	}
	gui.State.Q.mu.Unlock()

	gui.renderQTranscript()

	return nil
}

func (gui *Gui) renderQChats() {
	if !gui.qScreenActive() {
		return
	}

	gui.State.Q.mu.Lock()
	lines := make([]string, 0, len(gui.State.Q.chats))
	for _, chat := range gui.State.Q.chats {
		row := chat.started.Format("15:04") + " " + qChatMarker(chat) + chat.title()
		if turns := len(chat.turns); turns > 1 {
			row += fmt.Sprintf(" (%d)", turns)
		}
		lines = append(lines, row)
	}
	selected := gui.State.Q.selected
	gui.State.Q.mu.Unlock()

	content := strings.Join(lines, "\n")
	count := len(lines)

	// UpdateAsync preserves render order; Update could let an older history snapshot land last.
	gui.g.UpdateAsync(func(*gocui.Gui) error {
		view := gui.Views.QChats
		view.Title = fmt.Sprintf("Chats (%d)", count)
		if err := gui.setViewContent(view, content); err != nil {
			return err
		}
		gui.FocusY(selected, count, view)
		return nil
	})
}

func qChatMarker(chat *qChat) string {
	last := chat.last()
	switch {
	case last == nil:
		return ""
	case !last.done:
		return "… "
	case last.err != "":
		return "! "
	default:
		return ""
	}
}

func (gui *Gui) selectedQChatLocked() *qChat {
	if gui.State.Q.selected < 0 || gui.State.Q.selected >= len(gui.State.Q.chats) {
		return nil
	}

	return gui.State.Q.chats[gui.State.Q.selected]
}

func (gui *Gui) renderQOptions() error {
	return gui.renderOptionsMap(map[string]string{
		"enter":     "Ask",
		"tab":       "Next pane",
		"↑ ↓":       "Scroll (select chat in history)",
		"PgUp/PgDn": "Scroll",
		"click":     "Fold code",
		"ctrl+f":    "Fold all",
		"ctrl+p":    "Model",
		"ctrl+n":    "New chat",
		"esc":       "Dashboard",
	})
}
