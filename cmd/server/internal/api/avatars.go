package api

import (
	"errors"
	"net/http"

	"github.com/appthrust/appserver-go/appserver/httpx"
	"github.com/appthrust/appserver-go/appserver/link"
)

type rawBody struct {
	contentType  string
	data         []byte
	cacheControl string
}

func (a *API) avatar(s *request) (any, int, error) {
	body, contentType, err := a.directory.Avatar(s.r.Context(), s.bearer, s.r.PathValue("personId"))
	if err != nil {
		var denied *link.DeniedError
		var rejected *link.RejectedError
		switch {
		case errors.As(err, &denied):
			return nil, 0, httpx.Forbidden()
		case errors.As(err, &rejected) && rejected.Status == http.StatusNotFound:
			return nil, 0, httpx.NotFound()
		case errors.Is(err, link.ErrAbsent), errors.Is(err, link.ErrUnavailable):
			return nil, 0, directoryUnavailable()
		default:
			return nil, 0, err
		}
	}
	if len(body) > 1<<20 {
		return nil, 0, directoryUnavailable()
	}
	return rawBody{contentType: contentType, data: body, cacheControl: "private, max-age=86400"}, http.StatusOK, nil
}
