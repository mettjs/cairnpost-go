package cairnpost_test

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"

	cairnpost "github.com/mettjs/cairnpost-go"
)

func ExampleClient_Send() {
	client, err := cairnpost.New("https://mail.example.com", "cp_...")
	if err != nil {
		log.Fatal(err)
	}
	id, err := client.Send(context.Background(), cairnpost.Email{
		From:    "Example <hello@example.com>",
		To:      "ana@example.org",
		Subject: "Welcome",
		Text:    "Glad you are here.",
		// The same key and content, sent again, is the same email.
		IdempotencyKey: "welcome-user-42",
	})
	switch {
	case err == nil:
		fmt.Println("sent:", id)
	case cairnpost.MayHaveSent(err):
		// It is not known whether it went. Send again later with the same
		// IdempotencyKey, and only with it: a first try the server knows
		// went is answered with its id and not sent again.
	default:
		// Nothing was sent. An *Error says why and whether to try again.
		log.Print(err)
	}
}

func ExampleError() {
	client, _ := cairnpost.New("https://mail.example.com", "cp_...")
	_, err := client.Send(context.Background(), cairnpost.Email{
		From: "hello@example.com", To: "ana@example.org", Subject: "Welcome", Text: "Hi",
		IdempotencyKey: "welcome-user-42",
	})
	var problem *cairnpost.Error
	if errors.As(err, &problem) {
		switch {
		case problem.Name == cairnpost.ErrRecipientSuppress:
			// The address bounced for good or complained. Stop sending to
			// it, and do not retry. An EmailID here is an earlier try
			// that may have gone: Get says what became of it.
		case cairnpost.MayHaveSent(err):
			// It may have gone. Retry only with the same IdempotencyKey.
		case problem.Retryable():
			// Nothing was sent. Wait as long as the server asked, if it did.
			time.Sleep(max(problem.RetryAfter, time.Second))
		default:
			// The request is at fault. Trying again will not help.
		}
	}
}

func ExampleVerifier_Verify() {
	verifier, err := cairnpost.NewVerifier("whsec_...") // from CreateWebhook, or RotateWebhookSecret since
	if err != nil {
		log.Fatal(err)
	}
	http.HandleFunc("/webhooks/cairnpost", func(w http.ResponseWriter, r *http.Request) {
		// The signature is over the body exactly as it arrived.
		body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 1<<20))
		if err != nil {
			http.Error(w, "too large", http.StatusRequestEntityTooLarge)
			return
		}
		event, err := verifier.Verify(r.Header, body)
		if err != nil {
			http.Error(w, "not a CairnPost webhook", http.StatusUnauthorized)
			return
		}
		// event.ID is the same on every try: use it to act only once.
		if event.Type == cairnpost.EventBounced && event.Email.Bounce.Type == cairnpost.BouncePermanent {
			fmt.Println("the address is dead:", event.Email.To[0])
		}
		w.WriteHeader(http.StatusNoContent)
	})
}

func ExampleClient_AllSuppressions() {
	client, _ := cairnpost.New("https://mail.example.com", "cp_admin...")
	for s, err := range client.AllSuppressions(context.Background(), "example.com") {
		if err != nil {
			log.Fatal(err)
		}
		fmt.Println(s.Address, s.Reason)
	}
}
