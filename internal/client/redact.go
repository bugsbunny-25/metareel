package client

import (
	"fmt"
	"log/slog"
	"regexp"
)

// Some APIs (TMDB, MDBList) take the API key as a query parameter, and Go's
// HTTP errors include the full request URL. redactErr hides those values so
// keys never reach logs, task run logs or HTTP responses.
var secretParamRe = regexp.MustCompile(`(?i)\b((?:api_?key|access_token|token)=)[^&"\s]+`)

type redactedError struct{ err error }

func (e redactedError) Error() string {
	return secretParamRe.ReplaceAllString(e.err.Error(), "${1}REDACTED")
}

func (e redactedError) Unwrap() error { return e.err }

func redactErr(err error) error {
	if err == nil {
		return nil
	}
	return redactedError{err}
}

// redactingLogger is resty's logger: resty logs transport errors itself
// (with the full request URL, key included) before returning them, so its
// messages go through the same redaction to slog.
type redactingLogger struct{ log *slog.Logger }

func (l redactingLogger) msg(format string, v ...any) string {
	return secretParamRe.ReplaceAllString(fmt.Sprintf(format, v...), "${1}REDACTED")
}

func (l redactingLogger) Errorf(format string, v ...any) {
	l.log.Warn("http client: " + l.msg(format, v...))
}

func (l redactingLogger) Warnf(format string, v ...any) {
	l.log.Warn("http client: " + l.msg(format, v...))
}

func (l redactingLogger) Debugf(format string, v ...any) {
	l.log.Debug("http client: " + l.msg(format, v...))
}
