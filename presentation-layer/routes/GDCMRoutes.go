package server

import (
	"github.com/IntegratedBrainEnvironment/BrainNav_GO_BE/domain-layer/services"
	"github.com/IntegratedBrainEnvironment/BrainNav_GO_BE/presentation-layer/controller"
	"github.com/gorilla/mux"
)

func SetupGDCMRoutes(router *mux.Router, scansService services.ScansService) {
	gdcmController := controller.NewGDCMController(scansService)

	gdcmRouter := router.PathPrefix("/v1/gdcm").Subrouter()

	gdcmRouter.HandleFunc("/health", gdcmController.GDCMHealth).Methods("GET", "OPTIONS")

	gdcmRouter.HandleFunc("/upload", gdcmController.UploadDICOM).Methods("POST", "OPTIONS")
	gdcmRouter.HandleFunc("/compress", gdcmController.CompressDICOM).Methods("POST", "OPTIONS")
	gdcmRouter.HandleFunc("/decompress", gdcmController.DecompressDICOM).Methods("POST", "OPTIONS")
	gdcmRouter.HandleFunc("/info", gdcmController.GetDICOMInfo).Methods("POST", "OPTIONS")
	gdcmRouter.HandleFunc("/batch-compress", gdcmController.BatchCompressDICOM).Methods("POST", "OPTIONS")

	gdcmRouter.HandleFunc("/thumbnail", gdcmController.ThumbnailPNG).Methods("GET", "OPTIONS")
	gdcmRouter.HandleFunc("/render-frame", gdcmController.RenderFramePNG).Methods("GET", "OPTIONS")

	gdcmRouter.HandleFunc("/series/compress", gdcmController.CompressSeries).Methods("POST", "OPTIONS")
	gdcmRouter.HandleFunc("/series/decompress", gdcmController.DecompressSeries).Methods("POST", "OPTIONS")
	gdcmRouter.HandleFunc("/series/status", gdcmController.SeriesStatus).Methods("GET", "OPTIONS")

	gdcmRouter.HandleFunc("/download/{filename}", gdcmController.DownloadProcessedFile).Methods("GET", "OPTIONS")
	gdcmRouter.HandleFunc("/cleanup", gdcmController.CleanupFiles).Methods("DELETE", "OPTIONS")
}
