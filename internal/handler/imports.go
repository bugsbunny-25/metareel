package handler

import (
	"errors"
	"io"
	"net/http"

	"github.com/labstack/echo/v5"

	"github.com/bugsbunny-25/metareel/internal/service"
)

// UploadImport: POST /api/v1/me/imports?tz=&file_name=
// The body is the export zip itself (TV Time's GDPR data export or a Suite
// export); which one it is is worked out from the files inside. Answers 202:
// titles are matched to TMDB in the background, poll GET /imports/{id}.
func (h *Handler) UploadImport(c *echo.Context) error {
	body, err := io.ReadAll(io.LimitReader(c.Request().Body, service.MaxImportUploadBytes+1))
	if err != nil {
		if _, ok := errors.AsType[*http.MaxBytesError](err); ok {
			return c.JSON(http.StatusRequestEntityTooLarge, map[string]any{"error": "the upload is too large"})
		}
		return c.JSON(http.StatusBadRequest, map[string]any{"error": "couldn't read the upload"})
	}
	out, err := h.Imports.Upload(c.Request().Context(), userID(c), c.QueryParam("file_name"), c.QueryParam("tz"), body)
	return h.writeTracking(c, http.StatusAccepted, out, err)
}

// ListImports: GET /api/v1/me/imports
func (h *Handler) ListImports(c *echo.Context) error {
	out, err := h.Imports.List(c.Request().Context(), userID(c))
	return h.writeTracking(c, http.StatusOK, out, err)
}

// GetImport: GET /api/v1/me/imports/{id}
func (h *Handler) GetImport(c *echo.Context) error {
	id, err := parseID(c.Param("id"), "import id")
	if err != nil {
		return badRequest(c, err)
	}
	out, err := h.Imports.Get(c.Request().Context(), userID(c), id)
	return h.writeTracking(c, http.StatusOK, out, err)
}

// ListImportItems: GET /api/v1/me/imports/{id}/items?kind=&match=&limit=&offset=
func (h *Handler) ListImportItems(c *echo.Context) error {
	id, err := parseID(c.Param("id"), "import id")
	if err != nil {
		return badRequest(c, err)
	}
	limit, err := optionalInt(c.QueryParam("limit"), "limit")
	if err != nil {
		return badRequest(c, err)
	}
	offset, err := optionalInt(c.QueryParam("offset"), "offset")
	if err != nil {
		return badRequest(c, err)
	}
	out, err := h.Imports.Items(c.Request().Context(), userID(c), id, service.ImportItemsInput{
		Kind: c.QueryParam("kind"), Match: c.QueryParam("match"), Limit: limit, Offset: offset,
	})
	return h.writeTracking(c, http.StatusOK, out, err)
}

// UpdateImportItem: PUT /api/v1/me/imports/{id}/items/{item_id}
// {"tmdb_id": "123"} matches the title by hand; {"skip": true|false}
// leaves it out of (or back in) the import.
func (h *Handler) UpdateImportItem(c *echo.Context) error {
	id, err := parseID(c.Param("id"), "import id")
	if err != nil {
		return badRequest(c, err)
	}
	itemID, err := parseID(c.Param("item_id"), "item id")
	if err != nil {
		return badRequest(c, err)
	}
	var req service.ImportItemInput
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]any{"error": "invalid JSON body"})
	}
	out, err := h.Imports.SetItem(c.Request().Context(), userID(c), id, itemID, req)
	return h.writeTracking(c, http.StatusOK, out, err)
}

// CommitImport: POST /api/v1/me/imports/{id}/commit -- write a reviewed
// import (202; it runs in the background).
func (h *Handler) CommitImport(c *echo.Context) error {
	id, err := parseID(c.Param("id"), "import id")
	if err != nil {
		return badRequest(c, err)
	}
	out, err := h.Imports.Commit(c.Request().Context(), userID(c), id)
	return h.writeTracking(c, http.StatusAccepted, out, err)
}

// RetryImport: POST /api/v1/me/imports/{id}/retry -- resume a failed import.
func (h *Handler) RetryImport(c *echo.Context) error {
	id, err := parseID(c.Param("id"), "import id")
	if err != nil {
		return badRequest(c, err)
	}
	out, err := h.Imports.Retry(c.Request().Context(), userID(c), id)
	return h.writeTracking(c, http.StatusAccepted, out, err)
}

// UndoImport: POST /api/v1/me/imports/{id}/undo -- remove what an import
// wrote and put each title's state back.
func (h *Handler) UndoImport(c *echo.Context) error {
	id, err := parseID(c.Param("id"), "import id")
	if err != nil {
		return badRequest(c, err)
	}
	out, err := h.Imports.Undo(c.Request().Context(), userID(c), id)
	return h.writeTracking(c, http.StatusOK, out, err)
}

// DeleteImport: DELETE /api/v1/me/imports/{id} -- forget an import. Watches
// it wrote are kept.
func (h *Handler) DeleteImport(c *echo.Context) error {
	id, err := parseID(c.Param("id"), "import id")
	if err != nil {
		return badRequest(c, err)
	}
	if err := h.Imports.Delete(c.Request().Context(), userID(c), id); err != nil {
		return h.writeTracking(c, http.StatusOK, nil, err)
	}
	return c.NoContent(http.StatusNoContent)
}
