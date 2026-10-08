package client

import "regexp"

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
