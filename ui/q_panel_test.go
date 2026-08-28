package ui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/fatih/color"
	"github.com/jesseduffield/gocui"
	"github.com/noelruault/lazyaws/ui/layout"

	"github.com/noelruault/lazyaws/config"
)

// One headless screen per process on purpose: gocui keeps the screen in a package global and its event-poll goroutine outlives MainLoop, so a screen per test would have them racing each other.
var testScreen *gocui.Gui

func TestMain(m *testing.M) {
	// Colour is forced ONCE, before the loop goroutine exists: color.NoColor is a package global the render path reads on the loop goroutine, so a per-test toggle races whatever update a previous test left queued (caught by -race on main 688628d).
	os.Unsetenv("NO_COLOR")
	color.NoColor = false

	g, err := gocui.NewGui(gocui.NewGuiOpts{Headless: true, Width: 120, Height: 40})
	if err != nil {
		fmt.Fprintln(os.Stderr, "headless gocui:", err)
		os.Exit(1)
	}
	testScreen = g

	stopped := make(chan struct{})
	go func() {
		_ = g.MainLoop()
		close(stopped)
	}()

	code := m.Run()

	g.Update(func(*gocui.Gui) error { return gocui.ErrQuit })
	select {
	case <-stopped:
	case <-time.After(5 * time.Second):
		fmt.Fprintln(os.Stderr, "the main loop did not exit")
	}
	g.Close()

	os.Exit(code)
}

func newHeadlessGui(t *testing.T) (*Gui, *gocui.Gui) {
	t.Helper()

	// Shipped defaults keep the harness aligned with real installations.
	user := config.DefaultUserConfig()
	user.Chat = config.ChatConfig{Enabled: true, Provider: config.ProviderKiro, Model: config.DefaultChatModel}

	return newHeadlessGuiWithConfig(t, user)
}

func newHeadlessGuiWithConfig(t *testing.T, user config.UserConfig) (*Gui, *gocui.Gui) {
	t.Helper()

	g := testScreen

	gui, err := NewGui(&config.Config{User: user}, nil, make(chan error, 1))
	if err != nil {
		t.Fatalf("NewGui failed: %v", err)
	}
	gui.g = g

	// Screen setup must run on the gocui loop to avoid races.
	if err := ask(g, func() error {
		g.SetManager()
		if err := gui.createAllViews(); err != nil {
			return err
		}
		gui.setPanels()
		return gui.keybindings(g)
	}); err != nil {
		t.Fatalf("headless setup failed: %v", err)
	}

	return gui, g
}

func fakeQOnPath(t *testing.T, body string) {
	t.Helper()

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "kiro-cli"), []byte("#!/bin/sh\n"+body+"\n"), 0o755); err != nil {
		t.Fatalf("writing fake kiro-cli: %v", err)
	}
	t.Setenv("PATH", dir)
}

// printf '%s' with a single-quoted payload keeps the answer literal across newlines and needs no external command, which matters because the fake q runs with PATH pointing at its own directory only.
func fakeQAnswering(t *testing.T, answer string) {
	t.Helper()

	fakeQOnPath(t, "printf '%s' '"+strings.ReplaceAll(answer, "'", `'"'"'`)+"'")
}

func TestToggleQSwapsToTheQScreen(t *testing.T) {
	fakeQOnPath(t, "exit 0")
	gui, g := newHeadlessGui(t)

	run(t, g, gui.handleToggleQ)

	if !ask(g, gui.qScreenActive) {
		t.Error("Q.active = false, want the Q screen up")
	}
	if name := ask(g, func() string { return g.CurrentView().Name() }); name != "qInput" {
		t.Errorf("focused view = %q, want the question input", name)
	}

	windows := ask(g, func() map[string]layout.Dimensions { return gui.getWindowDimensions("", "") })
	for _, want := range []string{"qChats", "qInput", "main"} {
		if _, ok := windows[want]; !ok {
			t.Errorf("window %q is not laid out on the Q screen", want)
		}
	}
	for _, gone := range []string{"profile", "ecs", "ec2"} {
		if _, ok := windows[gone]; ok {
			t.Errorf("dashboard window %q is still laid out on the Q screen", gone)
		}
	}

	chats, conversation, input := windows["qChats"], windows["main"], windows["qInput"]
	if chats.X1 > conversation.X0 {
		t.Errorf("history ends at x=%d but the conversation starts at x=%d, want the history to its left", chats.X1, conversation.X0)
	}
	if input.Y0 < conversation.Y1 {
		t.Errorf("input starts at y=%d but the conversation ends at y=%d, want the input beneath it", input.Y0, conversation.Y1)
	}
	if input.X0 != conversation.X0 {
		t.Errorf("input starts at x=%d, want it aligned with the conversation at x=%d", input.X0, conversation.X0)
	}
	if chats.Y1 < input.Y1 {
		t.Errorf("history ends at y=%d, want it running the full height past the input at y=%d", chats.Y1, input.Y1)
	}
}

func TestQScreenIsOffByDefault(t *testing.T) {
	fakeQOnPath(t, `echo "should not run"`)
	gui, g := newHeadlessGuiWithConfig(t, config.DefaultUserConfig())

	if gui.qEnabled() {
		t.Fatal("the Amazon Q screen is enabled in the default config, want it off")
	}

	run(t, g, gui.handleToggleQ)

	waitForView(t, g, gui.Views.Confirmation, "off by default")
	if ask(g, gui.qScreenActive) {
		t.Error("Q.active = true while the feature is off")
	}
	if name := ask(g, func() string { return g.CurrentView().Name() }); name == "qInput" {
		t.Error("focus moved to the question input while the feature is off")
	}

	message := readView(g, gui.Views.Confirmation)
	for _, want := range []string{"Settings", "press o", "Amazon Q chat"} {
		if !strings.Contains(message, want) {
			t.Errorf("message = %q, want it to point at %q", message, want)
		}
	}

	run(t, g, func() error { return gui.askQ("sneak in") })
	if got := ask(g, func() int { return len(gui.State.Q.chats) }); got != 0 {
		t.Errorf("chats = %d, want none while the feature is off", got)
	}
}

func TestQScreenOpensOnceEnabled(t *testing.T) {
	fakeQOnPath(t, "exit 0")
	user := config.DefaultUserConfig()
	user.Chat.Enabled = true
	gui, g := newHeadlessGuiWithConfig(t, user)

	run(t, g, gui.handleToggleQ)

	if !ask(g, gui.qScreenActive) {
		t.Error("Q.active = false with the feature enabled")
	}
}

func TestToggleQWithoutCLI(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	gui, g := newHeadlessGui(t)

	run(t, g, gui.handleToggleQ)

	waitForView(t, g, gui.Views.Confirmation, "kiro-cli not found")
	if ask(g, gui.qScreenActive) {
		t.Error("Q.active = true with no q installed")
	}
}

func TestSwappingScreensKeepsBothStates(t *testing.T) {
	fakeQOnPath(t, `printf 'lists your buckets\n'`)
	gui, g := newHeadlessGui(t)

	run(t, g, func() error { return gui.switchFocus(gui.Views.EC2) })

	run(t, g, gui.handleToggleQ)
	run(t, g, func() error { return gui.askQ("how do I list buckets?") })
	waitForView(t, g, gui.Views.Main, "lists your buckets")

	run(t, g, gui.handleExitQ)

	if ask(g, gui.qScreenActive) {
		t.Error("Q.active = true after exiting")
	}
	if name := ask(g, func() string { return g.CurrentView().Name() }); name != "ec2" {
		t.Errorf("focused view = %q, want the dashboard panel we came from", name)
	}
	windows := ask(g, func() map[string]layout.Dimensions { return gui.getWindowDimensions("", "") })
	if _, ok := windows["profile"]; !ok {
		t.Error("dashboard windows are not laid out after exiting the Q screen")
	}
	if main := readView(g, gui.Views.Main); strings.Contains(main, "lists your buckets") {
		t.Errorf("main view = %q, want the transcript cleared on the way out", main)
	}

	run(t, g, gui.handleToggleQ)

	transcript := waitForView(t, g, gui.Views.Main, "lists your buckets")
	if !strings.Contains(transcript, "> how do I list buckets?") {
		t.Errorf("transcript = %q, want the earlier chat restored", transcript)
	}
	if got := ask(g, func() int { return len(gui.State.Q.chats) }); got != 1 {
		t.Errorf("chats = %d, want the history kept across the swap", got)
	}
}

func TestAskingTwiceContinuesTheSameConversation(t *testing.T) {
	// The fake prefixes argv[6] so echoed prompts stay distinguishable from answers.
	fakeQOnPath(t, `printf 'ANSWER >> %s\n' "$6"`)
	user := chatEnabledConfig()
	user.Chat.Provider = config.ProviderKiro
	gui, g := newHeadlessGuiWithConfig(t, user)

	run(t, g, gui.handleToggleQ)

	run(t, g, func() error { return gui.askQ("which ec2 are running?") })
	waitForView(t, g, gui.Views.Main, "ANSWER >> which ec2 are running?")

	run(t, g, func() error { return gui.askQ("and in the other region?") })
	conversation := waitForView(t, g, gui.Views.Main, "Your previous answer:")

	if got := ask(g, func() int { return len(gui.State.Q.chats) }); got != 1 {
		t.Errorf("chats = %d, want the second question to continue the first conversation", got)
	}
	if got := ask(g, func() int { return len(gui.State.Q.chats[0].turns) }); got != 2 {
		t.Errorf("turns = %d, want two", got)
	}
	if !strings.Contains(conversation, "> which ec2 are running?") {
		t.Errorf("conversation = %q, want the earlier turn still shown", conversation)
	}

	if !strings.Contains(conversation, "Question: which ec2 are running?") {
		t.Errorf("conversation = %q, want the earlier question handed to the backend", conversation)
	}

	chats := waitForView(t, g, gui.Views.QChats, "which ec2 are running?")
	if !strings.Contains(chats, "(2)") {
		t.Errorf("history = %q, want the turn count", chats)
	}
	if strings.Contains(chats, "and in the other region?") {
		t.Errorf("history = %q, want one row per conversation, not per question", chats)
	}
}

func TestNewConversationStartsAFreshChat(t *testing.T) {
	fakeQAnswering(t, "an answer\n")
	gui, g := newHeadlessGuiWithConfig(t, kiroChatConfig())

	run(t, g, gui.handleToggleQ)
	run(t, g, func() error { return gui.askQ("first question") })
	waitForView(t, g, gui.Views.Main, "an answer")

	run(t, g, gui.handleQNewChat)

	waitForView(t, g, gui.Views.Main, "Ask about this account")

	run(t, g, func() error { return gui.askQ("unrelated question") })
	waitForView(t, g, gui.Views.Main, "> unrelated question")

	if got := ask(g, func() int { return len(gui.State.Q.chats) }); got != 2 {
		t.Errorf("chats = %d, want a second conversation", got)
	}
	if got := ask(g, func() int { return len(gui.State.Q.chats[0].turns) }); got != 1 {
		t.Errorf("turns in the new conversation = %d, want just the new question", got)
	}
}

func TestChatHistoryKeepsEveryConversationAndSwitchesBetweenThem(t *testing.T) {
	fakeQOnPath(t, `printf 'ANSWER >> %s\n' "$6"`)
	gui, g := newHeadlessGuiWithConfig(t, kiroChatConfig())

	run(t, g, gui.handleToggleQ)

	for _, question := range []string{"first question", "second question"} {
		run(t, g, gui.handleQNewChat)
		run(t, g, func() error { return gui.askQ(question) })
		waitForView(t, g, gui.Views.Main, "> "+question)
	}

	// Wait for the SECOND question: the first is already on screen from the earlier render, so waiting for it can return before the newest render lands.
	chats := waitForView(t, g, gui.Views.QChats, "second question")
	if !strings.Contains(chats, "first question") {
		t.Errorf("history = %q, want both conversations", chats)
	}
	if strings.Index(chats, "second question") > strings.Index(chats, "first question") {
		t.Errorf("history = %q, want the newest first", chats)
	}
	if title := ask(g, func() string { return gui.Views.QChats.Title }); !strings.Contains(title, "2") {
		t.Errorf("history title = %q, want the count", title)
	}

	run(t, g, gui.handleQNextChat)
	transcript := waitForView(t, g, gui.Views.Main, "> first question")
	if strings.Contains(transcript, "> second question") {
		t.Errorf("transcript = %q, want only the selected conversation", transcript)
	}

	run(t, g, func() error { return gui.askQ("a follow-up") })
	waitForView(t, g, gui.Views.Main, "> a follow-up")
	if got := ask(g, func() int { return len(gui.State.Q.chats) }); got != 2 {
		t.Errorf("chats = %d, want the follow-up to join the selected conversation", got)
	}
}

func kiroChatConfig() config.UserConfig {
	user := chatEnabledConfig()
	user.Chat.Provider = config.ProviderKiro

	return user
}

func TestQRendersCLIFailure(t *testing.T) {
	fakeQOnPath(t, `printf 'not authenticated, run q login\n' >&2; exit 1`)
	gui, g := newHeadlessGui(t)

	run(t, g, gui.handleToggleQ)
	run(t, g, func() error { return gui.askQ("anything") })

	waitForView(t, g, gui.Views.Main, "not authenticated, run q login")
	waitForView(t, g, gui.Views.QChats, "! anything")
}

func TestQStreamDoesNotOverwriteTheDashboard(t *testing.T) {
	fakeQOnPath(t, `printf 'starting\n'; sleep 1; printf 'late answer\n'`)
	gui, g := newHeadlessGui(t)

	run(t, g, gui.handleToggleQ)
	run(t, g, func() error { return gui.askQ("something slow") })
	waitForView(t, g, gui.Views.Main, "starting")

	run(t, g, gui.handleExitQ)
	run(t, g, func() error { gui.reRenderStringMain("dashboard content"); return nil })
	waitForView(t, g, gui.Views.Main, "dashboard content")

	time.Sleep(1500 * time.Millisecond)
	if main := readView(g, gui.Views.Main); !strings.Contains(main, "dashboard content") || strings.Contains(main, "late answer") {
		t.Errorf("main view = %q, want the dashboard content left alone", main)
	}
}

func TestDashboardRenderDoesNotOverwriteTheConversation(t *testing.T) {
	fakeQOnPath(t, `printf 'the answer\n'`)
	gui, g := newHeadlessGui(t)

	run(t, g, gui.handleToggleQ)
	run(t, g, func() error { return gui.askQ("a question") })
	waitForView(t, g, gui.Views.Main, "the answer")

	run(t, g, func() error {
		gui.reRenderStringMain("stale ecs logs")
		gui.RenderStringMain("stale panel detail")
		return nil
	})

	time.Sleep(200 * time.Millisecond)
	main := readView(g, gui.Views.Main)
	if strings.Contains(main, "stale") {
		t.Errorf("main view = %q, want the conversation left alone", main)
	}
	if !strings.Contains(main, "the answer") {
		t.Errorf("main view = %q, want the conversation still there", main)
	}
}

func TestClickTogglesACodeBlock(t *testing.T) {
	fakeQAnswering(t, "here you go:\n```bash\n"+strings.Repeat("aws s3 ls\n", qFoldThreshold+1)+"```\n")
	gui, g := newHeadlessGui(t)

	run(t, g, gui.handleToggleQ)
	run(t, g, func() error { return gui.askQ("how do I list buckets?") })

	conversation := waitForView(t, g, gui.Views.Main, "click to expand")
	if strings.Contains(conversation, "aws s3 ls") {
		t.Errorf("conversation = %q, want the long block folded to start with", conversation)
	}

	headerRow := ask(g, func() int { return gui.State.Q.render.Folds[0].FirstRow })

	clickMain(t, g, gui, headerRow)

	conversation = waitForView(t, g, gui.Views.Main, "aws s3 ls")
	if !strings.Contains(conversation, "click to collapse") {
		t.Errorf("conversation = %q, want the open block offering to close", conversation)
	}
	if got := strings.Count(conversation, "aws s3 ls"); got != qFoldThreshold+1 {
		t.Errorf("body lines = %d, want %d", got, qFoldThreshold+1)
	}

	clickMain(t, g, gui, ask(g, func() int { return gui.State.Q.render.Folds[0].FirstRow }))

	conversation = waitForView(t, g, gui.Views.Main, "click to expand")
	if strings.Contains(conversation, "aws s3 ls") {
		t.Errorf("conversation = %q, want the block folded again", conversation)
	}
}

func TestToggleAllFolds(t *testing.T) {
	fakeQAnswering(t, "a:\n```bash\n"+strings.Repeat("cmd\n", qFoldThreshold+1)+"```\nb:\n```json\n"+strings.Repeat("{}\n", qFoldThreshold+1)+"```\n")
	gui, g := newHeadlessGui(t)

	run(t, g, gui.handleToggleQ)
	run(t, g, func() error { return gui.askQ("two blocks please") })
	waitForView(t, g, gui.Views.Main, "click to expand")

	run(t, g, gui.handleQToggleFolds)
	conversation := waitForView(t, g, gui.Views.Main, "cmd")
	if !strings.Contains(conversation, "{}") {
		t.Errorf("conversation = %q, want both blocks open", conversation)
	}

	run(t, g, gui.handleQToggleFolds)
	conversation = waitForView(t, g, gui.Views.Main, "click to expand")
	if strings.Contains(conversation, "cmd") || strings.Contains(conversation, "{}") {
		t.Errorf("conversation = %q, want both blocks folded", conversation)
	}
}

func TestFoldsAreKeptPerChat(t *testing.T) {
	fakeQAnswering(t, "```bash\n"+strings.Repeat("a command\n", qFoldThreshold+1)+"```\n")
	gui, g := newHeadlessGui(t)

	run(t, g, gui.handleToggleQ)
	run(t, g, func() error { return gui.askQ("first") })
	waitForView(t, g, gui.Views.Main, "click to expand")
	run(t, g, gui.handleQToggleFolds)
	waitForView(t, g, gui.Views.Main, "a command")

	run(t, g, func() error { return gui.askQ("second") })
	waitForView(t, g, gui.Views.Main, "click to expand")

	run(t, g, gui.handleQNextChat)
	conversation := waitForView(t, g, gui.Views.Main, "> first")
	if !strings.Contains(conversation, "click to collapse") {
		t.Errorf("conversation = %q, want the first chat's block still open", conversation)
	}
}

func TestChatBackendSelection(t *testing.T) {
	fakeQOnPath(t, `printf 'from the CLI\n'`)

	t.Run("kiro", func(t *testing.T) {
		user := config.DefaultUserConfig()
		user.Chat.Enabled = true
		user.Chat.Provider = config.ProviderKiro
		gui, g := newHeadlessGuiWithConfig(t, user)

		run(t, g, gui.handleToggleQ)
		if title := ask(g, func() string { return gui.Views.Main.Title }); !strings.Contains(title, "Kiro") {
			t.Errorf("title = %q, want it to name the Kiro backend", title)
		}

		run(t, g, func() error { return gui.askQ("who answers?") })
		waitForView(t, g, gui.Views.Main, "from the CLI")
	})

	t.Run("bedrock", func(t *testing.T) {
		user := config.DefaultUserConfig()
		user.Chat.Enabled = true
		gui, g := newHeadlessGuiWithConfig(t, user)

		if got := gui.chatProvider(); got != config.ProviderBedrock {
			t.Fatalf("provider = %q, want bedrock by default", got)
		}

		run(t, g, gui.handleToggleQ)
		title := ask(g, func() string { return gui.Views.Main.Title })
		if !strings.Contains(title, "Bedrock") || !strings.Contains(title, config.DefaultChatModel) {
			t.Errorf("title = %q, want it to name Bedrock and the model", title)
		}

		// No AWS session ensures this fails in Bedrock instead of invoking the CLI.
		run(t, g, func() error { return gui.askQ("who answers?") })
		conversation := waitForView(t, g, gui.Views.Main, "> who answers?")
		if strings.Contains(conversation, "from the CLI") {
			t.Errorf("conversation = %q, want the bedrock backend, not the CLI", conversation)
		}
		waitForView(t, g, gui.Views.Main, "chat")
	})
}

func TestBedrockBackendDoesNotNeedTheCLI(t *testing.T) {
	t.Setenv("PATH", t.TempDir())

	user := config.DefaultUserConfig()
	user.Chat.Enabled = true
	gui, g := newHeadlessGuiWithConfig(t, user)

	run(t, g, gui.handleToggleQ)

	if !ask(g, gui.qScreenActive) {
		t.Error("the chat screen refused to open on the bedrock backend with no CLI installed")
	}
}

// A popup left open must not survive a screen swap: it's hand-positioned, so nothing else would ever hide it, and its keys stop being routed once focus moves.
func TestScreenSwapsDismissPopups(t *testing.T) {
	gui, g := newHeadlessGuiWithConfig(t, chatEnabledConfig())

	run(t, g, func() error { return gui.createConfirmationPanel("Stuck?", "a popup", nil, nil) })
	waitForView(t, g, gui.Views.Confirmation, "a popup")

	run(t, g, gui.handleToggleQ)

	if ask(g, func() bool { return gui.Views.Confirmation.Visible }) {
		t.Error("the popup is still on screen after swapping to the chat")
	}

	run(t, g, func() error { return gui.createConfirmationPanel("Stuck?", "another popup", nil, nil) })
	waitForView(t, g, gui.Views.Confirmation, "another popup")
	run(t, g, gui.handleExitQ)
	if ask(g, func() bool { return gui.Views.Confirmation.Visible }) {
		t.Error("the popup is still on screen after leaving the chat")
	}

	run(t, g, func() error { return gui.createConfirmationPanel("Stuck?", "a third popup", nil, nil) })
	waitForView(t, g, gui.Views.Confirmation, "a third popup")
	run(t, g, gui.handleToggleSettings)
	if ask(g, func() bool { return gui.Views.Confirmation.Visible }) {
		t.Error("the popup is still on screen after swapping to settings")
	}
}

func TestInputNamesTheBackend(t *testing.T) {
	t.Run("bedrock", func(t *testing.T) {
		gui, g := newHeadlessGuiWithConfig(t, chatEnabledConfig())

		run(t, g, gui.handleToggleQ)

		if got := ask(g, func() string { return gui.Views.QInput.Title }); got != "Ask Bedrock" {
			t.Errorf("input title = %q, want %q", got, "Ask Bedrock")
		}
	})

	t.Run("kiro", func(t *testing.T) {
		fakeQOnPath(t, "exit 0")
		user := chatEnabledConfig()
		user.Chat.Provider = config.ProviderKiro
		gui, g := newHeadlessGuiWithConfig(t, user)

		run(t, g, gui.handleToggleQ)

		if got := ask(g, func() string { return gui.Views.QInput.Title }); got != "Ask Amazon Q" {
			t.Errorf("input title = %q, want %q", got, "Ask Amazon Q")
		}
	})
}

func chatEnabledConfig() config.UserConfig {
	user := config.DefaultUserConfig()
	user.Chat.Enabled = true

	return user
}

// The model picker is the way out of a model-specific failure, so it has to be reachable from the conversation and it has to persist the choice.
func TestPickingAModelFromTheConversation(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)

	gui, g := newHeadlessGuiWithConfig(t, chatEnabledConfig())
	run(t, g, func() error {
		gui.State.Settings.mu.Lock()
		gui.State.Settings.models = []string{"amazon.nova-micro-v1:0", "eu.anthropic.claude-sonnet-4-6-v1:0"}
		gui.State.Settings.mu.Unlock()
		return nil
	})
	run(t, g, gui.handleToggleQ)
	run(t, g, gui.handleQPickModel)

	labels := ask(g, func() []string {
		var out []string
		for _, item := range gui.Panels.Menu.List.GetItems() {
			out = append(out, item.Label)
		}
		return out
	})
	joined := strings.Join(labels, "|")
	if !strings.Contains(joined, "amazon.nova-micro-v1:0") {
		t.Errorf("menu = %q, want the discovered models", joined)
	}
	if !strings.Contains(joined, "* "+config.DefaultChatModel) {
		t.Errorf("menu = %q, want the current model marked", joined)
	}

	run(t, g, func() error { return gui.applyChatModel("amazon.nova-micro-v1:0") })

	if got := gui.chatModel(); got != "amazon.nova-micro-v1:0" {
		t.Errorf("model = %q, want the picked one", got)
	}
	saved, err := config.LoadUserConfig()
	if err != nil {
		t.Fatalf("LoadUserConfig() error = %v", err)
	}
	if saved.Chat.Model != "amazon.nova-micro-v1:0" {
		t.Errorf("saved model = %q, want the picked one", saved.Chat.Model)
	}
	if title := ask(g, func() string { return gui.Views.Main.Title }); !strings.Contains(title, "nova-micro") {
		t.Errorf("title = %q, want it to name the new model", title)
	}
}

func TestPickingAModelOnTheCLIBackend(t *testing.T) {
	fakeQOnPath(t, "exit 0")
	user := chatEnabledConfig()
	user.Chat.Provider = config.ProviderKiro
	gui, g := newHeadlessGuiWithConfig(t, user)

	run(t, g, gui.handleToggleQ)
	run(t, g, gui.handleQPickModel)

	waitForView(t, g, gui.Views.Confirmation, "chooses its own model")
}

func TestAFailedAnswerPointsAtTheModelPicker(t *testing.T) {
	gui, g := newHeadlessGuiWithConfig(t, chatEnabledConfig())

	run(t, g, gui.handleToggleQ)
	run(t, g, func() error { return gui.askQ("anything") })

	waitForView(t, g, gui.Views.Main, "ctrl+p")
}

// The conversation has to be scrollable from where you type; the editor swallows most keys, so the scroll keys must be bound on the input itself rather than left to the global bindings.
func TestScrollingTheConversationFromTheInput(t *testing.T) {
	fakeQAnswering(t, "first line\n"+strings.Repeat("filler\n", 200)+"last line\n")
	gui, g := newHeadlessGuiWithConfig(t, kiroChatConfig())

	run(t, g, gui.handleToggleQ)
	run(t, g, func() error { return gui.askQ("a long answer please") })
	waitForView(t, g, gui.Views.Main, "last line")

	// Autoscroll lands after content, so wait for origin instead of sampling early.
	waitFor(t, g, func() bool { _, oy := gui.Views.Main.Origin(); return oy > 0 }, "the conversation to scroll to the bottom")
	bottom := ask(g, func() int { _, oy := gui.Views.Main.Origin(); return oy })

	var boundUp, boundDown bool
	for _, b := range gui.GetInitialKeybindings() {
		if b.ViewName != "qInput" {
			continue
		}
		switch b.Key {
		case gocui.KeyArrowUp, gocui.KeyPgup:
			boundUp = true
		case gocui.KeyArrowDown, gocui.KeyPgdn:
			boundDown = true
		}
	}
	if !boundUp || !boundDown {
		t.Error("the input has no scroll bindings of its own, so the editor will swallow the keys")
	}

	run(t, g, gui.scrollUpMain)
	afterUp := ask(g, func() int { _, oy := gui.Views.Main.Origin(); return oy })
	if afterUp >= bottom {
		t.Errorf("origin = %d after scrolling up from %d, want it to move up", afterUp, bottom)
	}
	if ask(g, func() bool { return gui.Views.Main.Autoscroll }) {
		t.Error("autoscroll survived a scroll up, so the next render would yank the reader back down")
	}

	run(t, g, gui.scrollDownMain)
	afterDown := ask(g, func() int { _, oy := gui.Views.Main.Origin(); return oy })
	if afterDown <= afterUp {
		t.Errorf("origin = %d after scrolling down from %d, want it to move down", afterDown, afterUp)
	}
}

func TestQSubmitIgnoresEmptyQuestion(t *testing.T) {
	fakeQOnPath(t, `echo "should not run"`)
	gui, g := newHeadlessGui(t)

	run(t, g, gui.handleToggleQ)
	run(t, g, func() error {
		gui.Views.QInput.TextArea.TypeString("   ")
		return gui.handleQSubmit()
	})

	if got := ask(g, func() int { return len(gui.State.Q.chats) }); got != 0 {
		t.Errorf("chats = %d, want no chat started for an empty question", got)
	}
}

func TestQScreenTabCyclesThreePanes(t *testing.T) {
	fakeQOnPath(t, "exit 0")
	gui, g := newHeadlessGui(t)

	run(t, g, gui.handleToggleQ)

	focused := func() string { return ask(g, func() string { return g.CurrentView().Name() }) }

	if got := focused(); got != "qInput" {
		t.Fatalf("focused view = %q, want the input to start", got)
	}

	want := []string{"qChats", "main", "qInput"}
	for _, expected := range want {
		run(t, g, gui.handleQFocusNext)
		if got := focused(); got != expected {
			t.Fatalf("focused view = %q, want %q", got, expected)
		}
	}

	if !ask(g, func() bool { return gui.showsFocus(gui.Views.Main) }) {
		t.Error("the conversation wouldn't be highlighted when focused, so there'd be no way to tell where focus is")
	}

	run(t, g, gui.handleQFocusNext)
	run(t, g, gui.handleQFocusNext)
	if got := focused(); got != "main" {
		t.Fatalf("focused view = %q, want the conversation", got)
	}
	run(t, g, func() error { return gui.handleQMainEscape(g, gui.Views.Main) })
	if ask(g, gui.qScreenActive) {
		t.Error("esc from the conversation didn't leave the Q screen")
	}

	if ask(g, func() bool { return gui.showsFocus(gui.Views.Main) }) {
		t.Error("the dashboard's main panel would be highlighted")
	}
	if !ask(g, func() bool { return gui.showsFocus(gui.Views.EC2) }) {
		t.Error("a side panel wouldn't be highlighted when focused")
	}
}

func TestQScreenPanelJumpLeavesTheScreen(t *testing.T) {
	fakeQOnPath(t, "exit 0")
	gui, g := newHeadlessGui(t)

	run(t, g, gui.handleToggleQ)
	run(t, g, func() error { return gui.handleGoTo(gui.Views.EC2)(g, gui.Views.QInput) })

	if ask(g, gui.qScreenActive) {
		t.Error("Q.active = true after jumping to a dashboard panel")
	}
}

func TestQScreenKeybindings(t *testing.T) {
	fakeQOnPath(t, "exit 0")
	gui, _ := newHeadlessGui(t)

	wanted := []struct {
		viewName string
		key      interface{}
	}{
		// Read from the keymap rather than written literally, so rebinding the chat key stays a one-line edit in keymap.go.
		{"", gui.Keys.Get(KeyAmazonQ).Key},
		{"qInput", gocui.KeyEnter},
		{"qInput", gocui.KeyTab},
		{"qInput", gocui.KeyEsc},
		{"qChats", gocui.KeyTab},
		{"qChats", gocui.KeyEsc},
		{"qChats", gocui.KeyArrowDown},
	}

	for _, want := range wanted {
		found := false
		for _, b := range gui.GetInitialKeybindings() {
			if b.ViewName == want.viewName && b.Key == want.key {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("no binding for key %v on view %q", want.key, want.viewName)
		}
	}
}

// A real click can only land on a visible row, and gocui sets the clicked view's cursor before dispatching, so scrolling the row to the top and putting the cursor there is what the handler sees.
func clickMain(t *testing.T, g *gocui.Gui, gui *Gui, row int) {
	t.Helper()

	run(t, g, func() error {
		gui.Views.Main.Autoscroll = false
		if err := gui.Views.Main.SetOrigin(0, row); err != nil {
			return err
		}
		if err := gui.Views.Main.SetCursor(0, 0); err != nil {
			return err
		}
		return gui.handleQConversationClick()
	})
}

// run executes a handler on the main loop's goroutine, where gocui runs every keybinding handler; calling it straight from the test goroutine would race the loop's own rendering.
func run(t *testing.T, g *gocui.Gui, handler func() error) {
	t.Helper()

	if err := ask(g, handler); err != nil {
		t.Fatalf("handler failed: %v", err)
	}
}

// A timeout panics rather than returning the zero value: run() checks ask's error, so a silent zero reports success for a handler that never ran, and every assertion after it is meaningless anyway.
func ask[T any](g *gocui.Gui, read func() T) T {
	result := make(chan T, 1)
	g.Update(func(*gocui.Gui) error {
		result <- read()
		return nil
	})

	select {
	case value := <-result:
		return value
	case <-time.After(askTimeout):
		panic("the gocui loop did not run a queued read within " + askTimeout.String() + ": the test harness is wedged, not the code under test")
	}
}

// askTimeout is generous because it is a deadlock detector, not a performance assertion: a loaded CI box should not turn a passing suite red.
const askTimeout = 30 * time.Second

func waitForView(t *testing.T, g *gocui.Gui, view *gocui.View, want string) string {
	t.Helper()

	var last string
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		last = readView(g, view)
		if strings.Contains(last, want) {
			return last
		}
		time.Sleep(10 * time.Millisecond)
	}

	t.Fatalf("view %q never contained %q; got %q", view.Name(), want, last)
	return ""
}

func readView(g *gocui.Gui, view *gocui.View) string {
	return ask(g, view.Buffer)
}
