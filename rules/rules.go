package rules

import (
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"log"
	"mime"
	"mime/multipart"
	"mime/quotedprintable"
	"net/http"
	"net/mail"
	"regexp"
	"strings"
	"time"

	"github.com/emersion/go-imap"
	"github.com/emersion/go-imap/client"
)

type Rule interface {
	Message(*imap.Message)
	Action(ctx context.Context, client *client.Client) error
}

type Predicate interface {
	MatchMessage(*imap.Message) bool
}

type AndPredicate struct {
	Left  Predicate
	Right Predicate
}

func (p *AndPredicate) MatchMessage(msg *imap.Message) bool {
	return p.Left.MatchMessage(msg) && p.Right.MatchMessage(msg)
}

func (p *AndPredicate) String() string {
	return fmt.Sprintf("(%s) and (%s)", p.Left, p.Right)
}

type OrPredicate struct {
	Left  Predicate
	Right Predicate
}

func (p *OrPredicate) String() string {
	return fmt.Sprintf("(%s) or (%s)", p.Left, p.Right)
}

func (p *OrPredicate) MatchMessage(msg *imap.Message) bool {
	return p.Left.MatchMessage(msg) || p.Right.MatchMessage(msg)
}

func (p *NotPredicate) String() string {
	return fmt.Sprintf("not (%s)", p.Predicate)
}

type NotPredicate struct {
	Predicate Predicate
}

func (p *NotPredicate) MatchMessage(msg *imap.Message) bool {
	return !p.Predicate.MatchMessage(msg)
}

type StringPredicate interface {
	MatchString(string) bool
}

type StringEqualsPredicate string

func (p StringEqualsPredicate) MatchString(s string) bool {
	return string(p) == s
}

func (p StringEqualsPredicate) String() string {
	return fmt.Sprintf("= \"%s\"", string(p))
}

type FieldPredicate struct {
	Field     string
	Predicate StringPredicate
}

func NewFieldPredicate(field string, predicate StringPredicate) (*FieldPredicate, error) {
	switch field {
	case "to", "from", "subject":
		return &FieldPredicate{Field: field, Predicate: predicate}, nil
	default:
		return nil, fmt.Errorf("unknown field '%s'", field)
	}
}

func (p *FieldPredicate) MatchMessage(msg *imap.Message) bool {
	switch p.Field {
	case "to":
		for _, address := range msg.Envelope.To {
			if p.Predicate.MatchString(address.Address()) {
				return true
			}
		}
	case "from":
		for _, address := range msg.Envelope.From {
			if p.Predicate.MatchString(address.Address()) {
				return true
			}
		}
	case "subject":
		return p.Predicate.MatchString(msg.Envelope.Subject)
	}
	return false
}

func (p *FieldPredicate) String() string {
	switch p.Predicate.(type) {
	case *regexp.Regexp:
		return fmt.Sprintf("%s ~ \"%s\"", p.Field, p.Predicate)
	default:
		return fmt.Sprintf("%s %s", p.Field, p.Predicate)
	}
}

type MoveRule struct {
	Predicate Predicate
	Mailbox   string
	messages  *imap.SeqSet
}

func NewMoveRule(predicate Predicate, mailbox string) *MoveRule {
	return &MoveRule{
		Predicate: predicate,
		Mailbox:   mailbox,
		messages:  new(imap.SeqSet),
	}
}

func (r MoveRule) Message(msg *imap.Message) {
	if r.Predicate.MatchMessage(msg) {
		log.Printf("Moving '%s' to '%s'", msg.Envelope.Subject, r.Mailbox)
		r.messages.AddNum(msg.Uid)
	}
}

func (r *MoveRule) Action(ctx context.Context, client *client.Client) error {
	msgs := r.messages
	r.messages = new(imap.SeqSet)
	if msgs.Empty() {
		return nil
	}

	err := client.UidMove(msgs, r.Mailbox)
	if err != nil {
		return fmt.Errorf("move messages to mailbox `%s`: %w", r.Mailbox, err)
	}
	return nil
}

func (r *MoveRule) String() string {
	return fmt.Sprintf("if %s then move \"%s\"", r.Predicate, r.Mailbox)
}

type FlagRule struct {
	Predicate Predicate
	Flag      string
	messages  *imap.SeqSet
}

func NewFlagRule(predicate Predicate, flag string) *FlagRule {
	if flag == "" {
		flag = imap.FlaggedFlag
	}
	return &FlagRule{
		Predicate: predicate,
		Flag:      flag,
		messages:  new(imap.SeqSet),
	}
}

func (r FlagRule) Message(msg *imap.Message) {
	for _, flag := range msg.Flags {
		if flag == r.Flag {
			return // already flagged
		}
	}
	if r.Predicate.MatchMessage(msg) {
		log.Printf("Flagging message '%s' with '%s'", msg.Envelope.Subject, r.Flag)
		r.messages.AddNum(msg.Uid)
	}
}

func (r *FlagRule) Action(ctx context.Context, client *client.Client) error {
	msgs := r.messages
	r.messages = new(imap.SeqSet)
	if msgs.Empty() {
		return nil
	}

	flags := []interface{}{r.Flag}
	err := client.UidStore(msgs, imap.FormatFlagsOp(imap.AddFlags, true), flags, nil)
	if err != nil {
		return fmt.Errorf("flag messages with `%s`: %w", r.Flag, err)
	}
	return nil
}

func (r *FlagRule) String() string {
	return fmt.Sprintf("if %s then flag \"%s\"", r.Predicate, r.Flag)
}

type UnflagRule struct {
	Predicate Predicate
	Flag      string
	messages  *imap.SeqSet
}

func NewUnflagRule(predicate Predicate, flag string) *UnflagRule {
	if flag == "" {
		flag = imap.FlaggedFlag
	}
	return &UnflagRule{
		Predicate: predicate,
		Flag:      flag,
		messages:  new(imap.SeqSet),
	}
}

func (r UnflagRule) Message(msg *imap.Message) {
	for _, flag := range msg.Flags {
		if flag == r.Flag {
			return // already flagged
		}
	}
	if r.Predicate.MatchMessage(msg) {
		log.Printf("Unflagging message '%s' with '%s'", msg.Envelope.Subject, r.Flag)
		r.messages.AddNum(msg.Uid)
	}
}

func (r *UnflagRule) Action(ctx context.Context, client *client.Client) error {
	msgs := r.messages
	r.messages = new(imap.SeqSet)
	if msgs.Empty() {
		return nil
	}

	flags := []interface{}{r.Flag}
	err := client.UidStore(msgs, imap.FormatFlagsOp(imap.RemoveFlags, true), flags, nil)
	if err != nil {
		return fmt.Errorf("unflag messages with `%s`: %w", r.Flag, err)
	}
	return nil
}

func (r *UnflagRule) String() string {
	return fmt.Sprintf("if %s then unflag \"%s\"", r.Predicate, r.Flag)
}

type StreamRule struct {
	Predicate Predicate
	Content   StreamContent
	URL       string
	messages  *imap.SeqSet
	done      *imap.SeqSet
	client    *http.Client
}

type StreamContent string

const (
	StreamContentHTML   StreamContent = "html"
	StreamContentRFC822 StreamContent = "rfc822"
)

func NewStreamRule(predicate Predicate, content string, url string) *StreamRule {
	return &StreamRule{
		Predicate: predicate,
		Content:   StreamContent(content),
		URL:       url,
		messages:  new(imap.SeqSet),
		done:      new(imap.SeqSet), // this rule has processed this message previously
		client:    http.DefaultClient,
	}
}

func (r StreamRule) Message(msg *imap.Message) {
	if r.done.Contains(msg.Uid) {
		return
	}
	if r.Predicate.MatchMessage(msg) {
		log.Printf("Streaming '%s' to '%s'", msg.Envelope.Subject, r.URL)
		r.messages.AddNum(msg.Uid)
	}
}

const (
	RFC2822 string = "Mon, 02 Jan 2006 15:04:05 MST"
)

func (r *StreamRule) Action(ctx context.Context, client *client.Client) error {
	msgs := r.messages
	r.messages = new(imap.SeqSet)
	r.done.AddSet(msgs)
	if msgs.Empty() {
		return nil
	}

	messages := make(chan *imap.Message, 10)
	done := make(chan error, 1)
	go func() {
		done <- client.UidFetch(msgs, []imap.FetchItem{imap.FetchUid, "BODY[]"}, messages)
	}()

	for message := range messages {
		err := r.handleMessage(ctx, message)
		if err != nil {
			log.Printf("stream message `%s` to `%s`: %v", message.Envelope.Subject, r.URL, err)
		}
	}

	if err := <-done; err != nil {
		return fmt.Errorf("stream messages to `%s`: %w", r.URL, err)
	}

	return nil
}

func (r *StreamRule) handleMessage(ctx context.Context, message *imap.Message) error {
	var rfc822 io.Reader
	for _, v := range message.Body {
		if v == nil {
			continue
		}
		rfc822 = v
	}

	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	switch r.Content {
	case StreamContentRFC822:
		// Pass the email to the command verbatim
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, r.URL, rfc822)
		req.Header.Set("Content-Type", "message/rfc822")
		req.Header.Set("Accept", "application/json")
		if err != nil {
			return fmt.Errorf("stream messages to `%s`: construct post request: %w", r.URL, err)
		}
		resp, err := r.client.Do(req)
		if err != nil {
			return fmt.Errorf("stream messages to `%s`: do http request: %w", r.URL, err)
		}
		defer resp.Body.Close()
		if resp.StatusCode < 200 && resp.StatusCode >= 300 {
			return fmt.Errorf("stream messages to `%s`: error response: %d", r.URL, resp.StatusCode)
		}
	case StreamContentHTML:
		// Parse the email and find the HTML to pass to the command
		msg, err := mail.ReadMessage(rfc822)
		if err != nil {
			return fmt.Errorf("parse message: %w", err)
		}
		html, err := messageMIME(msg, "text/html")
		if err != nil {
			return fmt.Errorf("html of message %d: %w", message.Uid, err)
		}
		date, err := msg.Header.Date()
		if err != nil {
			return fmt.Errorf("parse date of message %d: %w", message.Uid, err)
		}
		dec := new(mime.WordDecoder)
		subject, err := dec.DecodeHeader(msg.Header.Get("Subject"))
		if err != nil {
			return fmt.Errorf("decode subject of message %d: %w", message.Uid, err)
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, r.URL, html)
		req.Header.Set("Content-Type", "message/rfc822")
		req.Header.Set("Accept", "application/json")
		req.Header.Set("X-Message-UUID", msg.Header.Get("X-Apple-UUID"))
		req.Header.Set("X-Message-Subject", subject)
		req.Header.Set("X-Message-Date-RFC3339", date.Format(time.RFC3339))
		req.Header.Set("X-Message-Date-RFC2822", date.Format(RFC2822))
		if err != nil {
			return fmt.Errorf("stream messages to `%s`: construct post request: %w", r.URL, err)
		}
		resp, err := r.client.Do(req)
		if err != nil {
			return fmt.Errorf("stream messages to `%s`: do http request: %w", r.URL, err)
		}
		defer resp.Body.Close()
		if resp.StatusCode < 200 && resp.StatusCode >= 300 {
			return fmt.Errorf("stream messages to `%s`: error response: %d", r.URL, resp.StatusCode)
		}
	}
	return nil
}

func (r *StreamRule) String() string {
	return fmt.Sprintf("if %s then stream %s \"%s\"", r.Predicate, r.Content, r.URL)
}

// Find and parse part of message
func messageMIME(message *mail.Message, contentType string) (io.Reader, error) {
	mediaType, params, err := mime.ParseMediaType(message.Header.Get("Content-Type"))
	if err != nil {
		return nil, fmt.Errorf("parse message content type: %w", err)
	}
	if !strings.HasPrefix(mediaType, "multipart/") {
		return nil, fmt.Errorf("expected multipart message but found %s", mediaType)
	}
	reader := multipart.NewReader(message.Body, params["boundary"])
	if reader == nil {
		return nil, fmt.Errorf("could not construct multipart reader for message")
	}
	for {
		part, err := reader.NextPart()
		if err != nil {
			return nil, fmt.Errorf("could not find %s part of message: %w", contentType, err)
		}
		mediaType, _, err := mime.ParseMediaType(part.Header.Get("Content-Type"))
		if err != nil {
			return nil, fmt.Errorf("parse multipart message part content type: %w", err)
		}
		if mediaType == contentType {
			enc := strings.ToLower(part.Header.Get("Content-Transfer-Encoding"))
			switch enc {
			case "base64":
				return base64.NewDecoder(base64.StdEncoding, part), nil
			case "quoted-printable":
				return quotedprintable.NewReader(part), nil
			default:
				return part, nil
			}
		}
	}
}
