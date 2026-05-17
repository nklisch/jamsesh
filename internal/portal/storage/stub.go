package storage

import "time"

// StubResponse builds the 410 Gone response body for an archived session.
// The Details.FinalBranchName field is omitted from JSON when nil.
func (s *service) StubResponse(rec *ArchivedRecord) ArchivedStub {
	msg := "This session was archived on " + rec.ArchivedAt.Format("2006-01-02") + "."
	if rec.FinalBranchName != nil {
		msg += " Final branch: " + *rec.FinalBranchName + "."
	}

	stub := ArchivedStub{
		Error:      "session.archived",
		Message:    msg,
		HTTPStatus: 410,
	}
	stub.Details.ArchivedAt = rec.ArchivedAt.Format(time.RFC3339)
	stub.Details.FinalBranchName = rec.FinalBranchName
	stub.Details.EndReason = rec.EndReason
	return stub
}
