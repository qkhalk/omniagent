// Package bot implements the Telegram bot frontend for OmniAgent.
package bot

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	tele "gopkg.in/telebot.v3"

	"github.com/omniagent/omniagent/internal/adapter"
	"github.com/omniagent/omniagent/internal/auth"
	"github.com/omniagent/omniagent/internal/config"
	"github.com/omniagent/omniagent/internal/log"
	"github.com/omniagent/omniagent/internal/session"
	"github.com/omniagent/omniagent/internal/store"
)

// Bot ties together the telebot instance and our services.
type Bot struct {
	cfg      *config.Config
	store    *store.Store
	auth     *auth.Service
	sessions *session.Manager
	tg       *tele.Bot

	// tg-user-id -> user token (in-memory cache; durable link would use DB)
	mu    sync.RWMutex
	links map[int64]string

	// tg-user-id -> current agent id
	agents map[int64]string

	// tg-user-id -> current session id
	cur map[int64]string

	// tg-user-id -> current project
	projects map[int64]string
}

// New constructs a Bot. If Telegram is not configured, returns nil.
func New(cfg *config.Config, st *store.Store, as *auth.Service, sm *session.Manager) (*Bot, error) {
	if cfg.TelegramToken == "" {
		return nil, nil
	}
	pref := tele.Settings{
		Token:  cfg.TelegramToken,
		Poller: &tele.LongPoller{Timeout: 10 * time.Second},
		Offline: false,
	}
	tg, err := tele.NewBot(pref)
	if err != nil {
		return nil, err
	}
	b := &Bot{
		cfg: cfg, store: st, auth: as, sessions: sm, tg: tg,
		links:    make(map[int64]string),
		agents:   make(map[int64]string),
		cur:      make(map[int64]string),
		projects: make(map[int64]string),
	}
	b.register()
	return b, nil
}

// Start launches the bot (long polling). Blocks until ctx is cancelled.
func (b *Bot) Start(ctx context.Context) {
	if b == nil {
		<-ctx.Done()
		return
	}
	go b.tg.Start()
	<-ctx.Done()
	b.tg.Stop()
}

// register wires all command and message handlers.
func (b *Bot) register() {
	b.tg.Handle("/start", b.handleStart)
	b.tg.Handle("/help", b.handleHelp)
	b.tg.Handle("/new", b.handleNew)
	b.tg.Handle("/agent", b.handleAgent)
	b.tg.Handle("/agents", b.handleAgents)
	b.tg.Handle("/repo", b.handleRepo)
	b.tg.Handle("/status", b.handleStatus)
	b.tg.Handle("/cancel", b.handleCancel)
	b.tg.Handle(tele.OnText, b.handleText)
}

// requireAuth returns the principal for a Telegram user, or sends a "please
// authenticate" message and nil.
func (b *Bot) requireAuth(c tele.Context) (*auth.Principal, bool) {
	uid := c.Sender().ID
	b.mu.RLock()
	tok := b.links[uid]
	b.mu.RUnlock()
	if tok == "" {
		_ = c.Send("🔒 Vui lòng xác thực bằng lệnh:\n\n/start <token>\n\n— Token được cấp trong file .env của OmniAgent.")
		return nil, false
	}
	p, err := b.auth.VerifyTelegram(context.Background(), tok, uid)
	if err != nil {
		_ = c.Send("⛔ " + err.Error())
		return nil, false
	}
	return p, true
}

// handleStart authenticates the user via /start <token>.
func (b *Bot) handleStart(c tele.Context) error {
	uid := c.Sender().ID
	args := strings.TrimSpace(c.Message().Payload)
	if args == "" {
		return c.Send("👋 Chào bạn! OmniAgent đang chờ token.\n\n" +
			"Gửi lệnh: /start <token>\n\n" +
			"Token nằm trong file .env của OmniAgent (OMNI_USER_TOKENS).")
	}
	p, err := b.auth.VerifyTelegram(context.Background(), args, uid)
	if err != nil {
		return c.Send("❌ Xác thực thất bại: " + err.Error())
	}
	b.mu.Lock()
	b.links[uid] = p.Token
	if _, ok := b.agents[uid]; !ok {
		b.agents[uid] = "claude"
	}
	b.mu.Unlock()
	_ = b.store.AddAudit(context.Background(), p.Token, "telegram",
		fmt.Sprintf("tg:%d", uid), "login", "", true)
	return c.Send(fmt.Sprintf("✅ Chào %s! Agent mặc định: claude.\n\n"+
		"Lệnh: /new /agent /repo /status /cancel /help\n\n"+
		"Bắt đầu gửi tin nhắn để chat với agent.", p.Label))
}

// handleHelp shows the command list.
func (b *Bot) handleHelp(c tele.Context) error {
	return c.Send("📖 *OmniAgent — Lệnh*\n\n" +
		"`/start <token>` — Xác thực\n" +
		"`/new` — Tạo phiên chat mới\n" +
		"`/agent <id>` — Đổi agent (claude, codex, gemini, antigravity, zcode)\n" +
		"`/agents` — Liệt kê agent khả dụng\n" +
		"`/repo <name>` — Đổi project\n" +
		"`/status` — Trạng thái hiện tại\n" +
		"`/cancel` — Huỷ tác vụ đang chạy\n" +
		"`/help` — Hiển thị trợ giúp này",
		tele.ModeMarkdown)
}

// handleNew creates a fresh session.
func (b *Bot) handleNew(c tele.Context) error {
	p, ok := b.requireAuth(c)
	if !ok {
		return nil
	}
	uid := c.Sender().ID
	b.mu.Lock()
	agent := b.agents[uid]
	if agent == "" {
		agent = "claude"
		b.agents[uid] = agent
	}
	project := b.projects[uid]
	b.mu.Unlock()
	sess := &store.Session{
		ID:        newID(),
		UserToken: p.Token,
		Channel:   "telegram",
		RemoteID:  fmt.Sprintf("tg:%d", uid),
		Agent:     agent,
		Project:   project,
	}
	if err := b.store.CreateSession(context.Background(), sess); err != nil {
		return c.Send("❌ " + err.Error())
	}
	b.mu.Lock()
	b.cur[uid] = sess.ID
	b.mu.Unlock()
	return c.Send(fmt.Sprintf("🆕 Phiên mới `%s` | agent=%s | repo=%s", sess.ID[:8], agent, defaultStr(project, "(root)")),
		tele.ModeMarkdown)
}

// handleAgent switches the current agent.
func (b *Bot) handleAgent(c tele.Context) error {
	if _, ok := b.requireAuth(c); !ok {
		return nil
	}
	arg := strings.TrimSpace(c.Message().Payload)
	if arg == "" {
		return c.Send("Cú pháp: /agent <id>  (claude | codex | gemini | antigravity | zcode)")
	}
	uid := c.Sender().ID
	b.mu.Lock()
	b.agents[uid] = arg
	b.cur[uid] = ""
	b.mu.Unlock()
	return c.Send("✅ Đã chuyển agent sang " + arg + ". Gửi /new để bắt đầu phiên mới.")
}

// handleAgents lists agents from the registry.
func (b *Bot) handleAgents(c tele.Context) error {
	if _, ok := b.requireAuth(c); !ok {
		return nil
	}
	// Use the adapter registry set via SetAdapterRegistry
	var lines []string
	for _, a := range registry.List() {
		available := "✗"
		if a.Available(context.Background()) {
			available = "✓"
		}
		lines = append(lines, fmt.Sprintf("%s  `%s`  %s", available, a.ID(), a.DisplayName()))
	}
	return c.Send("🤖 Agent khả dụng:\n\n"+strings.Join(lines, "\n"), tele.ModeMarkdown)
}

// handleRepo switches the current project.
func (b *Bot) handleRepo(c tele.Context) error {
	if _, ok := b.requireAuth(c); !ok {
		return nil
	}
	arg := strings.TrimSpace(c.Message().Payload)
	uid := c.Sender().ID
	b.mu.Lock()
	if arg == "" {
		// list projects
		b.mu.Unlock()
		dirs := listSubdirs(b.cfg.Workspace)
		if len(dirs) == 0 {
			return c.Send("Workspace trống: " + b.cfg.Workspace)
		}
		return c.Send("📁 Projects:\n" + strings.Join(dirs, "\n"))
	}
	b.projects[uid] = arg
	b.cur[uid] = ""
	b.mu.Unlock()
	return c.Send(fmt.Sprintf("✅ Project: %s. Gửi /new để tạo phiên mới.", arg))
}

// handleStatus shows the current state.
func (b *Bot) handleStatus(c tele.Context) error {
	p, ok := b.requireAuth(c)
	if !ok {
		return nil
	}
	uid := c.Sender().ID
	b.mu.RLock()
	agent := b.agents[uid]
	cur := b.cur[uid]
	project := b.projects[uid]
	b.mu.RUnlock()
	active := ""
	if cur != "" && b.sessions.IsRunning(cur) {
		active = " *(đang chạy)*"
	}
	return c.Send(fmt.Sprintf("👤 %s\n🤖 Agent: `%s`\n📁 Repo: %s\n🆔 Session: `%s`%s\n⚙️ Active runs: %d",
		p.Label, agent, defaultStr(project, "(root)"), defaultStr(cur, "—"), active, b.sessions.ActiveCount()),
		tele.ModeMarkdown)
}

// handleCancel aborts the active run.
func (b *Bot) handleCancel(c tele.Context) error {
	if _, ok := b.requireAuth(c); !ok {
		return nil
	}
	uid := c.Sender().ID
	b.mu.RLock()
	cur := b.cur[uid]
	b.mu.RUnlock()
	if cur == "" {
		return c.Send("Không có phiên nào đang chạy.")
	}
	if b.sessions.Cancel(cur) {
		return c.Send("🛑 Đã huỷ.")
	}
	return c.Send("Phiên không đang chạy.")
}

// handleText is the default chat handler.
func (b *Bot) handleText(c tele.Context) error {
	p, ok := b.requireAuth(c)
	if !ok {
		return nil
	}
	uid := c.Sender().ID
	b.mu.RLock()
	cur := b.cur[uid]
	agent := b.agents[uid]
	project := b.projects[uid]
	b.mu.RUnlock()

	if cur == "" {
		// auto-create a session
		sess := &store.Session{
			ID:        newID(),
			UserToken: p.Token,
			Channel:   "telegram",
			RemoteID:  fmt.Sprintf("tg:%d", uid),
			Agent:     defaultStr(agent, "claude"),
			Project:   project,
		}
		if err := b.store.CreateSession(context.Background(), sess); err != nil {
			return c.Send("❌ " + err.Error())
		}
		cur = sess.ID
		b.mu.Lock()
		b.cur[uid] = cur
		b.mu.Unlock()
	}
	if b.sessions.IsRunning(cur) {
		return c.Send("⏳ Agent đang chạy. Gửi /cancel để huỷ rồi thử lại.")
	}

	s, err := b.store.GetSession(context.Background(), cur)
	if err != nil || s == nil {
		return c.Send("❌ Phiên không tồn tại. Gửi /new.")
	}
	ad, ok := lookupAdapter(s.Agent)
	if !ok {
		return c.Send("❌ Agent không khả dụng: " + s.Agent)
	}
	projectDir, err := adapter.SafeJoin(b.cfg.Workspace, s.Project)
	if err != nil {
		return c.Send("❌ " + err.Error())
	}

	// Subscribe to events so we can push assistant text back to Telegram.
	sub := b.sessions.Subscribe(cur)
	go b.telegramPump(c, sub, cur)

	spec := adapter.Spec{
		Prompt:     c.Text(),
		ProjectDir: projectDir,
		Env:        b.cfg.AdapterEnv,
	}
	if err := b.sessions.Start(context.Background(), s, ad, spec); err != nil {
		b.sessions.Unsubscribe(cur, sub.ID)
		return c.Send("❌ " + err.Error())
	}
	_ = b.store.AddAudit(context.Background(), p.Token, "telegram", fmt.Sprintf("tg:%d", uid), "session.send", cur, true)
	return nil
}

// telegramPump streams session events into the Telegram chat.
func (b *Bot) telegramPump(c tele.Context, sub *session.Subscriber, sessID string) {
	defer b.sessions.Unsubscribe(sessID, sub.ID)
	var buf strings.Builder
	flush := func() {
		if buf.Len() == 0 {
			return
		}
		text := buf.String()
		buf.Reset()
		// chunk to 4000 chars to stay under Telegram limits
		for len(text) > 4000 {
			chunk := text[:4000]
			text = text[4000:]
			if _, err := b.tg.Send(c.Recipient(), chunk, tele.ModeMarkdown); err != nil {
				log.Warn("tg send", "err", err)
			}
		}
		if _, err := b.tg.Send(c.Recipient(), text, tele.ModeMarkdown); err != nil {
			log.Warn("tg send", "err", err)
		}
	}
	defer flush()
	for {
		select {
		case ev, ok := <-sub.Ch:
			if !ok {
				return
			}
			switch ev.Type {
			case "text":
				buf.WriteString(ev.Text)
				buf.WriteByte('\n')
			case "tool":
				if buf.Len() > 0 {
					flush()
				}
				_, _ = b.tg.Send(c.Recipient(), "🔧 "+ev.ToolName+": "+ev.ToolInput, tele.ModeMarkdown)
			case "error":
				_, _ = b.tg.Send(c.Recipient(), "⚠️ "+ev.Text)
			case "done":
				return
			}
		case <-sub.Done:
			return
		case <-time.After(2 * time.Second):
			// periodic flush so user sees incremental progress
			flush()
		}
	}
}

// ---------- small helpers ----------

func defaultStr(s, def string) string {
	if s == "" {
		return def
	}
	return s
}

// newID returns a 16-char hex id.
func newID() string { return randHex(16) }

// registry reference, set by main.go via SetAdapterRegistry
var registry *adapter.Registry

// SetAdapterRegistry wires the adapter registry so the bot can list adapters.
func SetAdapterRegistry(r *adapter.Registry) { registry = r }

// lookupAdapter returns the adapter with the given id.
func lookupAdapter(id string) (adapter.Adapter, bool) {
	if registry == nil {
		return nil, false
	}
	return registry.Get(id)
}

// listSubdirs returns immediate subdirectory names of root.
func listSubdirs(root string) []string {
	entries, err := osReadDir(root)
	if err != nil {
		return nil
	}
	var out []string
	for _, e := range entries {
		if e.IsDir() {
			out = append(out, e.Name())
		}
	}
	return out
}
