package main

import (
	"flag"
	"log"
	"os"

	"github.com/cptaffe/mailrules/parse"
	"github.com/cptaffe/mailrules/rules"
	"github.com/emersion/go-imap"
	"github.com/emersion/go-imap/client"
)

const mailbox = "INBOX"

var (
	hostFlag     = flag.String("host", "", "IMAP host:port")
	usernameFlag = flag.String("username", "", "IMAP login username")
	passwordFlag = flag.String("password", "", "IMAP login password")
	rulesFlag    = flag.String("rules", "", "rules file")
)

func main() {
	flag.Parse()

	log.Println("Parsing rules...")
	f, err := os.Open(*rulesFlag)
	if err != nil {
		log.Fatal(err)
	}
	rules, err := parse.Parse(f)
	if err != nil {
		log.Fatal(err)
	}

	log.Println("Connecting to server...")

	c, err := client.DialTLS(*hostFlag, nil)
	if err != nil {
		log.Fatal(err)
	}
	log.Println("Connected")

	// Don't forget to logout
	defer c.Logout()

	// Login
	if err := c.Login(*usernameFlag, *passwordFlag); err != nil {
		log.Fatal(err)
	}
	log.Println("Logged in")

	log.Println("Rules:")
	for _, rule := range rules {
		log.Printf("* %s", rule)
	}

	// Select INBOX
	mbox, err := c.Select(mailbox, false)
	if err != nil {
		log.Fatal(err)
	}

	for {
		processMailbox(c, mbox, rules)

		log.Println("Listening...")

		// Create a channel to receive mailbox updates
		updates := make(chan client.Update)
		c.Updates = updates

		// Start idling
		stop := make(chan struct{})
		done := make(chan error, 1)
		go func() {
			done <- c.Idle(stop, nil)
		}()

		// Listen for updates
		for {
			select {
			case update := <-updates:
				switch update := update.(type) {
				case *client.MailboxUpdate:
					if update.Mailbox.Name != mailbox {
						break
					}
					log.Println("Saw change to Inbox")

					// stop idling
					close(stop)
					close(updates)
					c.Updates = nil
				}
			case err := <-done:
				if err != nil {
					log.Fatal(err)
				}
				goto Process
			}
		}
	Process:
	}
}

func processMailbox(c *client.Client, mbox *imap.MailboxStatus, rules []rules.Rule) {
	seqset := new(imap.SeqSet)
	seqset.AddRange(1, 0)
	messages := make(chan *imap.Message, 10)
	done := make(chan error, 1)
	go func() {
		done <- c.UidFetch(seqset, []imap.FetchItem{imap.FetchUid, imap.FetchEnvelope}, messages)
	}()

	log.Println("Reading Inbox...")
	for msg := range messages {
		for _, rule := range rules {
			rule.Message(msg)
		}
	}

	// TODO: Multiple rules can match the same message and perform incompatible actions
	for _, rule := range rules {
		err := rule.Action(c)
		if err != nil {
			log.Println("Apply rule:", err)
		}
	}

	if err := <-done; err != nil {
		log.Fatal(err)
	}
}
