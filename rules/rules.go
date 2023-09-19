package rules

import (
	"fmt"
	"log"
	"regexp"

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
	return fmt.Sprintf("%s and %s", p.Left, p.Right)
}

type OrPredicate struct {
	Left  Predicate
	Right Predicate
}

func (p *OrPredicate) String() string {
	return fmt.Sprintf("%s or %s", p.Left, p.Right)
}

func (p *OrPredicate) MatchMessage(msg *imap.Message) bool {
	return p.Left.MatchMessage(msg) || p.Right.MatchMessage(msg)
}

func (p *NotPredicate) String() string {
	return fmt.Sprintf("not %s", p.Predicate)
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
		r.messages.AddNum(msg.SeqNum)
	}
}

func (r *MoveRule) Action(client *client.Client) error {
	msgs := r.messages
	r.messages = new(imap.SeqSet)
	if msgs.Empty() {
		return nil
	}

	err := client.Move(msgs, r.Mailbox)
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
		r.messages.AddNum(msg.SeqNum)
	}
}

func (r *FlagRule) Action(client *client.Client) error {
	msgs := r.messages
	r.messages = new(imap.SeqSet)
	if msgs.Empty() {
		return nil
	}

	flags := []interface{}{r.Flag}
	err := client.Store(msgs, imap.FormatFlagsOp(imap.AddFlags, true), flags, nil)
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
		r.messages.AddNum(msg.SeqNum)
	}
}

func (r *UnflagRule) Action(client *client.Client) error {
	msgs := r.messages
	r.messages = new(imap.SeqSet)
	if msgs.Empty() {
		return nil
	}

	flags := []interface{}{r.Flag}
	err := client.Store(msgs, imap.FormatFlagsOp(imap.RemoveFlags, true), flags, nil)
	if err != nil {
		return fmt.Errorf("unflag messages with `%s`: %w", r.Flag, err)
	}
	return nil
}

func (r *UnflagRule) String() string {
	return fmt.Sprintf("if %s then unflag \"%s\"", r.Predicate, r.Flag)
}
