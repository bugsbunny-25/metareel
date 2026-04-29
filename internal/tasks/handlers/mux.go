package handlers

import (
	"github.com/hibiken/asynq"
)

type Registrar interface {
	Register(mux *asynq.ServeMux)
}

func NewServeMux(registrars ...Registrar) *asynq.ServeMux {
	mux := asynq.NewServeMux()
	for _, r := range registrars {
		r.Register(mux)
	}
	return mux
}

