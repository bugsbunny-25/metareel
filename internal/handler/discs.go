package handler

import (
	"errors"
	"io"
	"net/http"
	"os"

	"github.com/labstack/echo/v5"

	"github.com/bugsbunny-25/metareel/internal/service"
)

// writeDiscs maps physical-media errors: 403 for a release the user doesn't
// own a copy of, the rest like tracking.
func (h *Handler) writeDiscs(c *echo.Context, status int, out any, err error) error {
	if forbidden, ok := errors.AsType[*service.ForbiddenError](err); ok {
		return c.JSON(http.StatusForbidden, map[string]any{"error": forbidden.Message})
	}
	return h.writeTracking(c, status, out, err)
}

// ListDiscs: GET /api/v1/me/discs?format=&q=&tmdb_kind=&tmdb_id=&sort=
func (h *Handler) ListDiscs(c *echo.Context) error {
	out, err := h.Discs.List(c.Request().Context(), userID(c), service.DiscListInput{
		MediaFormat: c.QueryParam("format"), Query: c.QueryParam("q"),
		TmdbKind: c.QueryParam("tmdb_kind"), TmdbID: c.QueryParam("tmdb_id"), Sort: c.QueryParam("sort"),
	})
	return h.writeDiscs(c, http.StatusOK, out, err)
}

// LookupDisc: GET /api/v1/me/discs/lookup?barcode=
func (h *Handler) LookupDisc(c *echo.Context) error {
	out, err := h.Discs.Lookup(c.Request().Context(), userID(c), c.QueryParam("barcode"))
	return h.writeDiscs(c, http.StatusOK, out, err)
}

// AddDisc: POST /api/v1/me/discs
func (h *Handler) AddDisc(c *echo.Context) error {
	var req service.AddDiscInput
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]any{"error": "invalid JSON body"})
	}
	out, err := h.Discs.Add(c.Request().Context(), userID(c), req)
	return h.writeDiscs(c, http.StatusCreated, out, err)
}

// GetDisc: GET /api/v1/me/discs/{id}
func (h *Handler) GetDisc(c *echo.Context) error {
	id, err := parseID(c.Param("id"), "disc id")
	if err != nil {
		return badRequest(c, err)
	}
	out, err := h.Discs.Get(c.Request().Context(), userID(c), id)
	return h.writeDiscs(c, http.StatusOK, out, err)
}

// UpdateDisc: PUT /api/v1/me/discs/{id} (the copy: price, retailer, ...)
func (h *Handler) UpdateDisc(c *echo.Context) error {
	id, err := parseID(c.Param("id"), "disc id")
	if err != nil {
		return badRequest(c, err)
	}
	var req service.DiscCopyInput
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]any{"error": "invalid JSON body"})
	}
	out, err := h.Discs.UpdateCopy(c.Request().Context(), userID(c), id, req)
	return h.writeDiscs(c, http.StatusOK, out, err)
}

// DeleteDisc: DELETE /api/v1/me/discs/{id}
func (h *Handler) DeleteDisc(c *echo.Context) error {
	id, err := parseID(c.Param("id"), "disc id")
	if err != nil {
		return badRequest(c, err)
	}
	if err := h.Discs.DeleteCopy(c.Request().Context(), userID(c), id); err != nil {
		return h.writeDiscs(c, http.StatusOK, nil, err)
	}
	return c.NoContent(http.StatusNoContent)
}

// GetDiscRelease: GET /api/v1/me/disc-releases/{id}
func (h *Handler) GetDiscRelease(c *echo.Context) error {
	id, err := parseID(c.Param("id"), "release id")
	if err != nil {
		return badRequest(c, err)
	}
	out, err := h.Discs.GetRelease(c.Request().Context(), userID(c), id)
	return h.writeDiscs(c, http.StatusOK, out, err)
}

// UpdateDiscRelease: PUT /api/v1/me/disc-releases/{id}
func (h *Handler) UpdateDiscRelease(c *echo.Context) error {
	id, err := parseID(c.Param("id"), "release id")
	if err != nil {
		return badRequest(c, err)
	}
	var req service.DiscReleaseInput
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]any{"error": "invalid JSON body"})
	}
	out, err := h.Discs.UpdateRelease(c.Request().Context(), userID(c), id, req)
	return h.writeDiscs(c, http.StatusOK, out, err)
}

// UploadDiscImage: POST /api/v1/me/disc-releases/{id}/images?kind=
// The body is the picture itself (JPEG, PNG, WebP or GIF).
func (h *Handler) UploadDiscImage(c *echo.Context) error {
	id, err := parseID(c.Param("id"), "release id")
	if err != nil {
		return badRequest(c, err)
	}
	body, err := io.ReadAll(io.LimitReader(c.Request().Body, service.MaxDiscImageBytes+1))
	if err != nil {
		if _, ok := errors.AsType[*http.MaxBytesError](err); ok {
			return c.JSON(http.StatusRequestEntityTooLarge, map[string]any{"error": "the picture is too large"})
		}
		return c.JSON(http.StatusBadRequest, map[string]any{"error": "couldn't read the upload"})
	}
	out, err := h.Discs.AddImage(c.Request().Context(), userID(c), id, c.QueryParam("kind"), body)
	return h.writeDiscs(c, http.StatusCreated, out, err)
}

// ImportDiscImage: POST /api/v1/me/disc-releases/{id}/images/import
// {"url": "...", "kind": "front"}: a picture a barcode lookup offered, or a
// TMDB poster.
func (h *Handler) ImportDiscImage(c *echo.Context) error {
	id, err := parseID(c.Param("id"), "release id")
	if err != nil {
		return badRequest(c, err)
	}
	var req struct {
		URL  string `json:"url"`
		Kind string `json:"kind"`
	}
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]any{"error": "invalid JSON body"})
	}
	out, err := h.Discs.ImportImage(c.Request().Context(), userID(c), id, req.Kind, req.URL)
	return h.writeDiscs(c, http.StatusCreated, out, err)
}

// UpdateDiscImage: PUT /api/v1/me/disc-releases/{id}/images/{image_id}
func (h *Handler) UpdateDiscImage(c *echo.Context) error {
	id, err := parseID(c.Param("id"), "release id")
	if err != nil {
		return badRequest(c, err)
	}
	imageID, err := parseID(c.Param("image_id"), "image id")
	if err != nil {
		return badRequest(c, err)
	}
	var req struct {
		Kind     string `json:"kind"`
		Position int64  `json:"position"`
	}
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]any{"error": "invalid JSON body"})
	}
	out, err := h.Discs.UpdateImage(c.Request().Context(), userID(c), id, imageID, req.Kind, req.Position)
	return h.writeDiscs(c, http.StatusOK, out, err)
}

// DeleteDiscImage: DELETE /api/v1/me/disc-releases/{id}/images/{image_id}
func (h *Handler) DeleteDiscImage(c *echo.Context) error {
	id, err := parseID(c.Param("id"), "release id")
	if err != nil {
		return badRequest(c, err)
	}
	imageID, err := parseID(c.Param("image_id"), "image id")
	if err != nil {
		return badRequest(c, err)
	}
	if err := h.Discs.DeleteImage(c.Request().Context(), userID(c), id, imageID); err != nil {
		return h.writeDiscs(c, http.StatusOK, nil, err)
	}
	return c.NoContent(http.StatusNoContent)
}

// SearchDiscCollections: GET /api/v1/me/disc-collections?q=
// TMDB collections with their films, to fill a box set in one go.
func (h *Handler) SearchDiscCollections(c *echo.Context) error {
	out, err := h.Discs.SearchCollections(c.Request().Context(), c.QueryParam("q"), 5)
	return h.writeDiscs(c, http.StatusOK, map[string]any{"results": out}, err)
}

// GetDiscImage: GET /api/v1/disc-images/{name}
// A picture's name is its content hash, so it is cached for good.
func (h *Handler) GetDiscImage(c *echo.Context) error {
	f, contentType, err := h.Discs.OpenImage(c.Param("name"))
	if errors.Is(err, os.ErrNotExist) {
		return c.JSON(http.StatusNotFound, map[string]any{"error": "no such picture"})
	}
	if err != nil {
		return writeData(c, nil, err)
	}
	defer f.Close()
	hd := c.Response().Header()
	hd.Set("X-Content-Type-Options", "nosniff")
	hd.Set("Content-Security-Policy", "default-src 'none'; sandbox")
	hd.Set("Cache-Control", "private, max-age=31536000, immutable")
	return c.Stream(http.StatusOK, contentType, f)
}
