package server

import (
	"net/http"

	"github.com/scout-kit/fine-print/internal/api"
	"github.com/scout-kit/fine-print/internal/db"
)

func RegisterRoutes(mux *http.ServeMux, handlers *api.Handlers, queries *db.Queries, sseHub *SSEHub) {
	adminAuth := AdminAuthMiddleware(queries)

	// Health check
	mux.HandleFunc("GET /api/health", handlers.Health)

	// First-run setup (public — only mutates while the wizard hasn't run)
	mux.HandleFunc("GET /api/setup/status", handlers.SetupStatus)
	mux.HandleFunc("POST /api/setup/complete", handlers.CompleteSetup)

	// Guest routes
	mux.HandleFunc("POST /api/photos", handlers.UploadPhoto)
	mux.HandleFunc("GET /api/photos/{id}/status", handlers.PhotoStatus)
	mux.HandleFunc("GET /api/photos/{id}/preview", handlers.PhotoPreview)
	mux.HandleFunc("GET /api/photos/{id}/download/original", handlers.DownloadOriginal)
	mux.HandleFunc("GET /api/photos/{id}/download/rendered", handlers.DownloadRendered)
	mux.HandleFunc("GET /api/photos/{id}/render", handlers.RenderPreview)
	mux.HandleFunc("POST /api/photos/{id}/transform", handlers.SaveTransform)
	mux.HandleFunc("GET /api/photos/{id}/edits", handlers.GetEdits)
	mux.HandleFunc("POST /api/photos/{id}/edits", handlers.SaveEdits)
	mux.HandleFunc("POST /api/photos/{id}/booth-print", handlers.BoothPrint)
	mux.HandleFunc("DELETE /api/photos/{id}", handlers.DeleteOwnPhoto)
	mux.HandleFunc("GET /api/projects", handlers.ListProjectsPublic)
	mux.HandleFunc("GET /api/projects/{id}", handlers.GetProjectPublic)
	mux.HandleFunc("GET /api/projects/s/{slug}", handlers.GetProjectBySlug)
	mux.HandleFunc("GET /api/gallery", handlers.Gallery)
	mux.HandleFunc("GET /api/qr", handlers.QRCode)
	mux.HandleFunc("GET /api/qr/project/{id}", handlers.ProjectQRCode)
	mux.HandleFunc("GET /api/fonts", handlers.ListFonts)
	mux.HandleFunc("GET /api/fonts/available", handlers.ListSystemFonts)
	mux.HandleFunc("GET /api/fonts/{id}", handlers.ServeFont)

	// Guest SSE
	mux.Handle("GET /api/events", sseHub.Handler(false))

	// Admin auth routes
	mux.HandleFunc("POST /api/admin/login", handlers.AdminLogin)
	mux.HandleFunc("GET /api/admin/session", handlers.AdminSession)
	mux.HandleFunc("POST /api/admin/logout", handlers.AdminLogout)

	// Admin protected routes
	mux.Handle("GET /api/admin/photos", adminAuth(http.HandlerFunc(handlers.ListPhotos)))
	mux.Handle("POST /api/admin/photos/{id}/approve", adminAuth(http.HandlerFunc(handlers.ApprovePhoto)))
	mux.Handle("POST /api/admin/photos/{id}/reject", adminAuth(http.HandlerFunc(handlers.RejectPhoto)))
	mux.Handle("POST /api/admin/photos/{id}/unapprove", adminAuth(http.HandlerFunc(handlers.UnapprovePhoto)))
	mux.Handle("POST /api/admin/photos/{id}/override", adminAuth(http.HandlerFunc(handlers.OverridePhoto)))
	mux.Handle("DELETE /api/admin/photos/{id}", adminAuth(http.HandlerFunc(handlers.DeletePhoto)))
	mux.Handle("POST /api/admin/photos/{id}/reprint", adminAuth(http.HandlerFunc(handlers.ReprintPhoto)))

	// Print queue
	mux.Handle("GET /api/admin/queue", adminAuth(http.HandlerFunc(handlers.ListQueue)))
	mux.Handle("POST /api/admin/queue/pause", adminAuth(http.HandlerFunc(handlers.PauseQueue)))
	mux.Handle("POST /api/admin/queue/resume", adminAuth(http.HandlerFunc(handlers.ResumeQueue)))
	mux.Handle("POST /api/admin/queue/{id}/retry", adminAuth(http.HandlerFunc(handlers.RetryJob)))
	mux.Handle("POST /api/admin/queue/{id}/cancel", adminAuth(http.HandlerFunc(handlers.CancelJob)))

	// Projects
	mux.Handle("GET /api/admin/projects", adminAuth(http.HandlerFunc(handlers.ListProjects)))
	mux.Handle("POST /api/admin/projects", adminAuth(http.HandlerFunc(handlers.CreateProject)))
	mux.Handle("GET /api/admin/projects/{id}", adminAuth(http.HandlerFunc(handlers.GetProject)))
	mux.Handle("PUT /api/admin/projects/{id}", adminAuth(http.HandlerFunc(handlers.UpdateProject)))
	mux.Handle("DELETE /api/admin/projects/{id}", adminAuth(http.HandlerFunc(handlers.DeleteProject)))
	mux.Handle("POST /api/admin/projects/{id}/overlay", adminAuth(http.HandlerFunc(handlers.UploadOverlay)))
	mux.Handle("POST /api/admin/projects/{id}/text-overlay", adminAuth(http.HandlerFunc(handlers.CreateTextOverlay)))
	mux.Handle("POST /api/admin/projects/{id}/copy-template", adminAuth(http.HandlerFunc(handlers.CopyTemplateOrientation)))
	mux.Handle("POST /api/admin/projects/{id}/copy", adminAuth(http.HandlerFunc(handlers.CopyProject)))

	// Overlays
	mux.Handle("GET /api/admin/overlays/{id}", adminAuth(http.HandlerFunc(handlers.ServeOverlay)))
	mux.Handle("PUT /api/admin/overlays/{id}", adminAuth(http.HandlerFunc(handlers.UpdateOverlayPosition)))
	mux.Handle("DELETE /api/admin/overlays/{id}", adminAuth(http.HandlerFunc(handlers.DeleteOverlay)))

	// Text Overlays
	mux.Handle("PUT /api/admin/text-overlays/{id}", adminAuth(http.HandlerFunc(handlers.UpdateTextOverlayHandler)))
	mux.Handle("DELETE /api/admin/text-overlays/{id}", adminAuth(http.HandlerFunc(handlers.DeleteTextOverlayHandler)))

	// Fonts
	mux.Handle("POST /api/admin/fonts", adminAuth(http.HandlerFunc(handlers.UploadFont)))
	mux.Handle("DELETE /api/admin/fonts/{id}", adminAuth(http.HandlerFunc(handlers.DeleteFont)))

	// Printers
	mux.Handle("GET /api/admin/printers", adminAuth(http.HandlerFunc(handlers.ListPrinters)))
	mux.Handle("POST /api/admin/printers/sync", adminAuth(http.HandlerFunc(handlers.SyncPrinters)))
	mux.Handle("PUT /api/admin/printers/enabled", adminAuth(http.HandlerFunc(handlers.UpdatePrinterEnabled)))
	mux.Handle("GET /api/admin/printers/settings", adminAuth(http.HandlerFunc(handlers.GetPrinterSettings)))
	mux.Handle("PUT /api/admin/printers/mode", adminAuth(http.HandlerFunc(handlers.UpdatePrinterMode)))
	mux.Handle("POST /api/admin/printers/test", adminAuth(http.HandlerFunc(handlers.TestPrint)))

	// Settings
	mux.Handle("GET /api/admin/settings", adminAuth(http.HandlerFunc(handlers.GetSettings)))
	mux.Handle("PUT /api/admin/settings", adminAuth(http.HandlerFunc(handlers.UpdateSettings)))
	mux.Handle("POST /api/admin/settings/password", adminAuth(http.HandlerFunc(handlers.ChangeAdminPassword)))
	mux.Handle("POST /api/admin/settings/restart", adminAuth(http.HandlerFunc(handlers.RestartService)))

	// Export
	mux.Handle("GET /api/admin/export/{project_id}", adminAuth(http.HandlerFunc(handlers.ExportProject)))
	mux.Handle("POST /api/admin/export/photos", adminAuth(http.HandlerFunc(handlers.ExportPhotos)))

	// Admin SSE
	mux.Handle("GET /api/admin/events", adminAuth(sseHub.Handler(true)))
}
