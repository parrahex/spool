package platform

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

	"github.com/slack-go/slack"
	"github.com/slack-go/slack/slackevents"
	"github.com/slack-go/slack/socketmode"
)

// Slack implements Platform over the Slack socket-mode API
type Slack struct {
	api    *slack.Client
	socket *socketmode.Client
	token  string
}

// NewSlack needs two tokens: the app-level token for the socket-mode
// connection and the bot token for posting messages
func NewSlack(appToken, botToken string) *Slack {
	api := slack.New(botToken, slack.OptionAppLevelToken(appToken))
	return &Slack{
		api:    api,
		socket: socketmode.New(api),
		token:  botToken,
	}
}

// Listen starts the socket-mode connection; dispatch runs on a separate
// goroutine so handler work never blocks event processing
func (s *Slack) Listen(ctx context.Context, handler HandlerFunc) error {
	go s.dispatch(ctx, handler)
	return s.socket.RunContext(ctx)
}

// dispatch must Ack every event promptly, otherwise Slack retries it after
// a timeout; each incoming event is handled asynchronously so a slow
// handler does not stall the event stream
func (s *Slack) dispatch(ctx context.Context, handler HandlerFunc) {
	for evt := range s.socket.Events {
		if evt.Type != socketmode.EventTypeEventsAPI {
			continue
		}
		s.socket.Ack(*evt.Request)
		msg, ok := extractMessage(evt)
		if !ok {
			continue
		}
		go handler(ctx, msg)
	}
}

// extractMessage converts a Slack event into a platform Message and
// filters out events the bot must ignore: messages from other bots, non-DM
// channels, and subtypes other than file shares
func extractMessage(evt socketmode.Event) (Message, bool) {
	apiEvt, ok := evt.Data.(slackevents.EventsAPIEvent)
	if !ok || apiEvt.Type != slackevents.CallbackEvent {
		return Message{}, false
	}

	switch ev := apiEvt.InnerEvent.Data.(type) {
	case *slackevents.MessageEvent:
		if ev.BotID != "" {
			return Message{}, false
		}
		if ev.ChannelType != "im" {
			return Message{}, false
		}
		if ev.SubType != "" && ev.SubType != "file_share" {
			return Message{}, false
		}

		msg := Message{
			Text:    ev.Text,
			Channel: ev.Channel,
			Thread:  ev.TimeStamp,
			Sender:  ev.User,
		}
		if ev.ThreadTimeStamp != "" {
			msg.Thread = ev.ThreadTimeStamp
		}
		if ev.Message != nil {
			for _, f := range ev.Message.Files {
				msg.Files = append(msg.Files, File{ID: f.ID, Name: f.Name})
			}
		}
		return msg, true

	case *slackevents.AppMentionEvent:
		msg := Message{
			Text:    stripMention(ev.Text),
			Channel: ev.Channel,
			Thread:  ev.TimeStamp,
			Sender:  ev.User,
		}
		if ev.ThreadTimeStamp != "" {
			msg.Thread = ev.ThreadTimeStamp
		}
		for _, f := range ev.Files {
			msg.Files = append(msg.Files, File{ID: f.ID, Name: f.Name})
		}
		return msg, true
	}

	return Message{}, false
}

// stripMention removes the leading <@USERID> mention; Slack wraps mentions
// in angle brackets instead of sending plain text
func stripMention(text string) string {
	for strings.HasPrefix(text, "<@") {
		idx := strings.Index(text, ">")
		if idx == -1 {
			break
		}
		text = strings.TrimSpace(text[idx+1:])
	}
	return text
}

// Reply posts into the original thread so the conversation stays coherent
func (s *Slack) Reply(ctx context.Context, msg Message, text string) error {
	_, _, err := s.api.PostMessageContext(ctx, msg.Channel,
		slack.MsgOptionText(text, false),
		slack.MsgOptionTS(msg.Thread),
	)
	return err
}

// Download fetches a file via its private URL, which requires the bot
// token as a Bearer credential — public download URLs expire
func (s *Slack) Download(ctx context.Context, f File, dest string) error {
	info, _, _, err := s.api.GetFileInfoContext(ctx, f.ID, 0, 0)
	if err != nil {
		return fmt.Errorf("file info: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, info.URLPrivateDownload, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+s.token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("download: %w", err)
	}
	defer resp.Body.Close()
	out, err := os.Create(dest)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, resp.Body)
	return err
}
