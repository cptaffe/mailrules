package rules

import (
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"log"
	"mime"
	"mime/multipart"
	"mime/quotedprintable"
	"net/mail"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"time"

	"github.com/emersion/go-imap"
	"github.com/emersion/go-imap/client"
)

type Rule interface {
	Message(*imap.Message)
	Action(client *client.Client) error
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

func (r *MoveRule) Action(client *client.Client) error {
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

func (r *FlagRule) Action(client *client.Client) error {
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

func (r *UnflagRule) Action(client *client.Client) error {
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
	Command   string
	messages  *imap.SeqSet
}

func NewStreamRule(predicate Predicate, command string) *StreamRule {
	return &StreamRule{
		Predicate: predicate,
		Command:   command,
		messages:  new(imap.SeqSet),
	}
}

func (r StreamRule) Message(msg *imap.Message) {
	if r.Predicate.MatchMessage(msg) {
		log.Printf("Streaming '%s' to '%s'", msg.Envelope.Subject, r.Command)
		r.messages.AddNum(msg.Uid)
	}
}

const (
	RFC2822 string = "Mon, 02 Jan 2006 15:04:05 MST"
)

func (r *StreamRule) Action(client *client.Client) error {
	msgs := r.messages
	r.messages = new(imap.SeqSet)
	if msgs.Empty() {
		return nil
	}

	messages := make(chan *imap.Message, 10)
	done := make(chan error, 1)
	go func() {
		done <- client.UidFetch(msgs, []imap.FetchItem{imap.FetchUid, imap.FetchRFC822Header, imap.FetchRFC822Text}, messages)
	}()

	var errs []error
	for message := range messages {
		msg, err := messageParse(message)
		if err != nil {
			errs = append(errs, fmt.Errorf("parse message %d: %w", message.Uid, err))
			continue
		}
		html, err := messageMIME(msg, "text/html")
		if err != nil {
			errs = append(errs, fmt.Errorf("html of message %d: %w", message.Uid, err))
			continue
		}
		cmd := exec.Command("bash", "-c", r.Command)
		date, err := msg.Header.Date()
		if err != nil {
			errs = append(errs, fmt.Errorf("parse date of message %d: %w", message.Uid, err))
		}
		dec := new(mime.WordDecoder)
		subject, err := dec.DecodeHeader(msg.Header.Get("Subject"))
		if err != nil {
			errs = append(errs, fmt.Errorf("decode subject of message %d: %w", message.Uid, err))
			subject = msg.Header.Get("Subject") // Use un-decoded subject
		}
		cmd.Env = append(
			os.Environ(),
			[]string{
				"message_uuid=" + msg.Header.Get("X-Apple-UUID"),
				"message_subject=" + subject,
				"message_date_rfc3339=" + date.Format(time.RFC3339),
				"message_date_rfc2822=" + date.Format(RFC2822),
			}...,
		)
		cmd.Stdin = html
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			errs = append(errs, fmt.Errorf("stream messages to `%s`: %w", r.Command, err))
			continue
		}
	}
	if len(errs) != 0 {
		return errors.Join(errs...)
	}

	if err := <-done; err != nil {
		return fmt.Errorf("stream messages to `%s`: %w", r.Command, err)
	}

	return nil
}

func (r *StreamRule) String() string {
	return fmt.Sprintf("if %s then stream \"%s\"", r.Predicate, r.Command)
}

func messageParse(message *imap.Message) (*mail.Message, error) {
	var header io.Reader
	var body io.Reader
	for k, v := range message.Body {
		if v == nil {
			continue
		}
		switch k.Specifier {
		case "HEADER":
			header = v
		case "TEXT":
			body = v
		}
	}
	if header == nil || body == nil {
		return nil, fmt.Errorf("message %d missing header, body, or both", message.Uid)
	}
	msg, err := mail.ReadMessage(io.MultiReader(header, body))
	if err != nil {
		return nil, fmt.Errorf("parse message: %w", err)
	}
	return msg, nil
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
