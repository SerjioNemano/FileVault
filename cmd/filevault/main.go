package main

import (
	"context"
	"filevault/internal/config"
	"filevault/internal/db"
	"filevault/internal/files"
	"filevault/internal/storage"
	"filevault/web"
	"log"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

func main() {
	log.Printf("Started filevault")
	conf, err := config.Load()
	if err != nil {
		log.Fatal(err)
	}

	ctx := context.Background()
	conn, err := db.Connect(ctx, conf.DatabaseURL)
	if err != nil {
		log.Fatal(err)
	}
	defer conn.Close()

	st, err := storage.NewStorage(conf.StorageDir)
	if err != nil {
		log.Fatal(err)
	}

	if err := db.Migrate(conf.DatabaseURL); err != nil {
		log.Fatal(err)
	}

	repo := files.NewPgRepository(conn)

	previewWorker := files.NewPreviewWorker(repo, st)
	go previewWorker.Run(ctx)

	service := files.NewService(repo, repo, st, previewWorker)
	handler := files.NewHandler(service, conf.MaxUploadBytes)

	r := chi.NewRouter()
	// HEAD обслуживается GET-хендлером. Статика — только r.Get, не r.Handle:
	// Handle берёт все методы, и GetHead решил бы, что HEAD-маршрут уже есть (/*)
	r.Use(middleware.GetHead)
	r.Get("/*", http.FileServerFS(web.Static()).ServeHTTP)
	r.Get("/api/dirs", handler.GetDirs)
	r.Post("/api/dirs", handler.CreateDir)
	r.Post("/api/dirs/{id}/rename", handler.RenameDir)
	r.Delete("/api/dirs/{id}", handler.DeleteDir)
	r.Get("/api/dirs/{id}/files", handler.GetDirFiles)
	r.Post("/api/dirs/{id}/files", handler.AddFileToDir)
	r.Get("/api/dirs/{id}/count_files", handler.GetDirCountFiles)
	r.Get("/api/files/{id}/original", handler.GetFileByID)
	r.Get("/api/files/{id}/preview", handler.GetFilePreview)
	r.Post("/api/files/{id}/rename", handler.RenameFile)
	r.Post("/api/files/{id}/move", handler.MoveFile)
	r.Delete("/api/files/{id}", handler.DeleteFile)

	log.Fatal(http.ListenAndServe(":"+conf.Port, r))
}
