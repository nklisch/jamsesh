// export_test.go exposes internal functions for use in package tests.
// This file is compiled ONLY when running tests (it is in package senders,
// not senders_test).
package senders

import (
	"fmt"

	sendgrid "github.com/sendgrid/sendgrid-go"
	sgrest "github.com/sendgrid/rest"
)

// ClassifyPostmarkErrorCode exposes classifyPostmarkErrorCode for test assertions.
func ClassifyPostmarkErrorCode(code int64, cause error) error {
	return classifyPostmarkErrorCode(code, cause)
}

// NewSendGridSenderForTest builds a sendgridSender that targets the given URL
// (e.g. a mock httptest.Server). Useful for injecting a test server without
// modifying production code.
func NewSendGridSenderForTest(url, from string) (*sendgridSender, error) {
	if from == "" {
		return nil, fmt.Errorf("from must not be empty")
	}
	req := sgrest.Request{
		BaseURL: url,
		Method:  "POST",
		Headers: map[string]string{
			"Authorization": "Bearer test-key",
			"User-Agent":    "sendgrid-go/test",
			"Accept":        "application/json",
		},
	}
	return &sendgridSender{
		client: &sendgrid.Client{Request: req},
		from:   from,
	}, nil
}
