package server

import (
	"github.com/IntegratedBrainEnvironment/BrainNav_GO_BE/presentation-layer/controller"
	"github.com/gorilla/mux"
)

func RegisterScansRoutes(router *mux.Router, controller *controller.ScansController) {
	router.HandleFunc("/patients/{patientID:[0-9]+}/scans", controller.GetAllScans).Methods("GET", "OPTIONS")
	router.HandleFunc("/scans/{id:[0-9]+}", controller.GetScanByID).Methods("GET", "OPTIONS")
	router.HandleFunc("/profiles/{profileFirstUid}/scans", controller.GetAllScans).Methods("GET", "OPTIONS")
	router.HandleFunc("/scans/{sid}", controller.GetScanBySid).Methods("GET", "OPTIONS")
	router.HandleFunc("/profiles/{profileFirstUid}/scans", controller.CreateScan).Methods("POST", "OPTIONS")
	router.HandleFunc("/scans/{sid}", controller.UpdateScan).Methods("PUT", "OPTIONS")
	router.HandleFunc("/scans/{sid}", controller.DeleteScan).Methods("DELETE", "OPTIONS")
	router.HandleFunc("/scans/{sid}/init", controller.InitUpload).Methods("POST", "OPTIONS")
	router.HandleFunc("/scans/{sid}/init-upload", controller.InitUpload).Methods("POST", "OPTIONS")
	router.HandleFunc("/scans/{sid}/status", controller.GetStatus).Methods("GET", "OPTIONS")
	router.HandleFunc("/scans/{sid}/upload-file", controller.UploadFile).Methods("POST", "OPTIONS")
	router.HandleFunc("/scans/{sid}/upload-zip", controller.UploadZip).Methods("POST", "OPTIONS")
	router.HandleFunc("/scans/{sid}/upload-batch", controller.UploadBatch).Methods("POST", "OPTIONS")
	router.HandleFunc("/scans/{sid}/upload", controller.UploadFile).Methods("POST", "OPTIONS")
	router.HandleFunc("/scans/{sid}/complete", controller.CompleteUpload).Methods("POST", "OPTIONS")
	router.HandleFunc("/scans/{sid}/abort", controller.AbortUpload).Methods("POST", "OPTIONS")
	router.HandleFunc("/scans/{sid}/files", controller.ListFiles).Methods("GET", "OPTIONS")
	router.HandleFunc("/scans/{sid}/files/{path:.*}", controller.DownloadFile).Methods("GET", "OPTIONS")

	router.HandleFunc("/scans/{sid}/decompress/{filename}", controller.DecompressFile).Methods("GET", "OPTIONS")
	router.HandleFunc("/scans/{sid}/decompress-batch", controller.DecompressBatch).Methods("POST", "OPTIONS")
	router.HandleFunc("/cache/stats", controller.GetCacheStats).Methods("GET", "OPTIONS")
	router.HandleFunc("/cache/clear/{sid}", controller.ClearScanCache).Methods("DELETE", "OPTIONS")
}
