package controller

import (
	"encoding/json"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/IntegratedBrainEnvironment/BrainNav_GO_BE/domain-layer/services"
	"github.com/gorilla/mux"
)

func isSafeFilename(name string) bool {
	if name == "" {
		return false
	}
	if name == "." || name == ".." || filepath.Base(name) != name {
		return false
	}
	for _, r := range name {
		if (r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '.' || r == '_' || r == '-' {
			continue
		}
		return false
	}
	return true
}

func fileExists(path string) bool {
	if path == "" {
		return false
	}
	if info, err := os.Stat(path); err == nil {
		return !info.IsDir()
	}
	return false
}

type GDCMController struct {
	BaseController
	gdcmService  *services.GDCMService
	scansService services.ScansService
}

type DICOMUploadRequest struct {
	Compress bool `json:"compress"`
}

type DICOMProcessRequest struct {
	InputPath  string `json:"input_path"`
	OutputPath string `json:"output_path"`
	Operation  string `json:"operation"`
}

type DICOMBatchRequest struct {
	Files []struct {
		Input  string `json:"input"`
		Output string `json:"output"`
	} `json:"files"`
}

func NewGDCMController(scansSvc services.ScansService) *GDCMController {
	return &GDCMController{
		gdcmService:  services.NewGDCMService(),
		scansService: scansSvc,
	}
}

func (gc *GDCMController) respondError(w http.ResponseWriter, statusCode int, message string, err error) {
	logMessage := message
	if err != nil {
		logMessage += ": " + err.Error()
	}
	log.Printf("ERROR: %s", logMessage)

	errorResponse := map[string]interface{}{
		"error":  message,
		"status": statusCode,
	}
	if err != nil {
		errorResponse["details"] = err.Error()
	}

	gc.BaseController.RespondWithJSON(w, statusCode, errorResponse)
}

func (gc *GDCMController) respondJSON(w http.ResponseWriter, statusCode int, data interface{}) {
	gc.BaseController.RespondWithJSON(w, statusCode, data)
}

func (gc *GDCMController) RegisterGDCMRoutes(router *mux.Router) {
	router.HandleFunc("/gdcm/health", gc.GDCMHealth).Methods("GET")

	router.HandleFunc("/gdcm/upload", gc.UploadDICOM).Methods("POST")
	router.HandleFunc("/gdcm/compress", gc.CompressDICOM).Methods("POST")
	router.HandleFunc("/gdcm/decompress", gc.DecompressDICOM).Methods("POST")
	router.HandleFunc("/gdcm/info", gc.GetDICOMInfo).Methods("POST")
	router.HandleFunc("/gdcm/batch-compress", gc.BatchCompressDICOM).Methods("POST")

	router.HandleFunc("/gdcm/thumbnail", gc.ThumbnailPNG).Methods("GET")
	router.HandleFunc("/gdcm/render-frame", gc.RenderFramePNG).Methods("GET")

	router.HandleFunc("/gdcm/series/compress", gc.CompressSeries).Methods("POST")
	router.HandleFunc("/gdcm/series/decompress", gc.DecompressSeries).Methods("POST")
	router.HandleFunc("/gdcm/series/status", gc.SeriesStatus).Methods("GET")

	router.HandleFunc("/gdcm/download/{filename}", gc.DownloadProcessedFile).Methods("GET")
	router.HandleFunc("/gdcm/cleanup", gc.CleanupFiles).Methods("DELETE")
}

func (gc *GDCMController) CompressSeries(w http.ResponseWriter, r *http.Request) {
	gc.respondJSON(w, http.StatusGone, map[string]string{"message": "compression removed"})
}

func (gc *GDCMController) DecompressSeries(w http.ResponseWriter, r *http.Request) {
	gc.respondJSON(w, http.StatusGone, map[string]string{"message": "decompression removed"})
}

func (gc *GDCMController) SeriesStatus(w http.ResponseWriter, r *http.Request) {
	sid := strings.TrimSpace(r.URL.Query().Get("sid"))
	if sid == "" {
		gc.respondError(w, http.StatusBadRequest, "sid required", nil)
		return
	}
	scan, err := gc.scansService.GetScanBySid(r.Context(), sid)
	if err != nil {
		gc.respondError(w, http.StatusNotFound, "scan not found", err)
		return
	}
	gc.respondJSON(w, http.StatusOK, map[string]interface{}{
		"sid":           sid,
		"upload_status": scan.UploadStatus,
		"received":      scan.ReceivedFileCount,
		"expected":      scan.ExpectedFileCount,
	})
}

func (gc *GDCMController) GDCMHealth(w http.ResponseWriter, r *http.Request) {
	log.Println("INFO: Checking GDCM service health")

	health, err := gc.gdcmService.HealthCheck()
	if err != nil {
		log.Printf("ERROR: GDCM health check failed: %v", err)
		gc.respondError(w, http.StatusServiceUnavailable, "GDCM service unavailable", err)
		return
	}

	gc.respondJSON(w, http.StatusOK, map[string]interface{}{
		"gdcm_service":   health,
		"backend_status": "healthy",
		"integration":    "active",
		"timestamp":      health.Timestamp,
	})
}

func (gc *GDCMController) UploadDICOM(w http.ResponseWriter, r *http.Request) {
	log.Println("INFO: Processing DICOM file upload")

	err := r.ParseMultipartForm(32 << 20)
	if err != nil {
		log.Printf("ERROR: Failed to parse multipart form: %v", err)
		gc.respondError(w, http.StatusBadRequest, "Failed to parse form data", err)
		return
	}

	file, header, err := r.FormFile("dicom_file")
	if err != nil {
		log.Printf("ERROR: Failed to get file from form: %v", err)
		gc.respondError(w, http.StatusBadRequest, "No DICOM file provided", err)
		return
	}
	defer file.Close()

	compressStr := r.FormValue("compress")
	compress, _ := strconv.ParseBool(compressStr)
	finalStr := r.FormValue("final")
	finalFlag, _ := strconv.ParseBool(finalStr)
	sid := r.FormValue("sid")
	if sid == "" {
		sid = r.Header.Get("X-Series-Uid")
	}
	sid = strings.TrimSpace(sid)

	log.Printf("INFO: Processing uploaded file - Name: %s, Size: %d, Compress: %t, Final: %t",
		header.Filename, header.Size, compress, finalFlag)

	ext := filepath.Ext(header.Filename)
	if ext != ".dcm" && ext != ".dicom" {
		gc.respondError(w, http.StatusBadRequest, "Invalid file type. Only .dcm and .dicom files are allowed", nil)
		return
	}

	result, err := gc.gdcmService.UploadAndProcessDICOM(file, header.Filename, compress, sid, finalFlag)
	if err != nil {
		log.Printf("ERROR: DICOM processing failed: %v", err)
		gc.respondError(w, http.StatusInternalServerError, "DICOM processing failed", err)
		return
	}

	log.Printf("INFO: DICOM processing completed successfully - Status: %s", result.Status)
	gc.respondJSON(w, http.StatusOK, map[string]interface{}{
		"message":    "DICOM file stored successfully",
		"filename":   header.Filename,
		"compressed": false,
		"sid":        sid,
		"result":     result,
		"final":      finalFlag,
	})
}

func (gc *GDCMController) CompressDICOM(w http.ResponseWriter, r *http.Request) {
	log.Println("INFO: Processing DICOM compression request")

	var request DICOMProcessRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		log.Printf("ERROR: Failed to decode compression request: %v", err)
		gc.respondError(w, http.StatusBadRequest, "Invalid request format", err)
		return
	}

	if request.InputPath == "" || request.OutputPath == "" {
		gc.respondError(w, http.StatusBadRequest, "Missing required fields: input_path, output_path", nil)
		return
	}

	log.Printf("INFO: Compressing DICOM - Input: %s, Output: %s", request.InputPath, request.OutputPath)

	gc.respondJSON(w, http.StatusGone, map[string]interface{}{"message": "compression removed"})
}

func (gc *GDCMController) DecompressDICOM(w http.ResponseWriter, r *http.Request) {
	log.Println("INFO: Processing DICOM decompression request")

	var request DICOMProcessRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		log.Printf("ERROR: Failed to decode decompression request: %v", err)
		gc.respondError(w, http.StatusBadRequest, "Invalid request format", err)
		return
	}

	if request.InputPath == "" || request.OutputPath == "" {
		gc.respondError(w, http.StatusBadRequest, "Missing required fields: input_path, output_path", nil)
		return
	}

	log.Printf("INFO: Decompressing DICOM - Input: %s, Output: %s", request.InputPath, request.OutputPath)

	gc.respondJSON(w, http.StatusGone, map[string]interface{}{"message": "decompression removed"})
}

func (gc *GDCMController) GetDICOMInfo(w http.ResponseWriter, r *http.Request) {
	log.Println("INFO: Processing DICOM info extraction request")

	var request struct {
		FilePath string `json:"file_path"`
	}
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		log.Printf("ERROR: Failed to decode info request: %v", err)
		gc.respondError(w, http.StatusBadRequest, "Invalid request format", err)
		return
	}

	if request.FilePath == "" {
		gc.respondError(w, http.StatusBadRequest, "Missing required field: file_path", nil)
		return
	}

	log.Printf("INFO: Extracting DICOM info - File: %s", request.FilePath)

	result, err := gc.gdcmService.GetDICOMInfo(request.FilePath)
	if err != nil {
		log.Printf("ERROR: DICOM info extraction failed: %v", err)
		gc.respondError(w, http.StatusInternalServerError, "DICOM info extraction failed", err)
		return
	}

	log.Printf("INFO: DICOM info extraction completed successfully")
	gc.respondJSON(w, http.StatusOK, map[string]interface{}{
		"message": "DICOM info extracted successfully",
		"result":  result,
	})
}

func (gc *GDCMController) BatchCompressDICOM(w http.ResponseWriter, r *http.Request) {
	log.Println("INFO: Processing batch DICOM compression request")

	var request DICOMBatchRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		log.Printf("ERROR: Failed to decode batch request: %v", err)
		gc.respondError(w, http.StatusBadRequest, "Invalid request format", err)
		return
	}

	if len(request.Files) == 0 {
		gc.respondError(w, http.StatusBadRequest, "No files provided for batch processing", nil)
		return
	}

	log.Printf("INFO: Starting batch compression - File count: %d", len(request.Files))

	files := make([]struct {
		Input  string
		Output string
	}, len(request.Files))

	for i, file := range request.Files {
		if file.Input == "" || file.Output == "" {
			gc.respondError(w, http.StatusBadRequest, "Missing input or output path in file entry", nil)
			return
		}
		files[i].Input = file.Input
		files[i].Output = file.Output
	}

	gc.respondJSON(w, http.StatusGone, map[string]interface{}{"message": "compression removed"})
}

func (gc *GDCMController) DownloadProcessedFile(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	filename := vars["filename"]

	if filename == "" {
		gc.respondError(w, http.StatusBadRequest, "Filename is required", nil)
		return
	}

	log.Printf("INFO: Download request for file: %s", filename)

	if !isSafeFilename(filename) {
		gc.respondError(w, http.StatusBadRequest, "Invalid filename", nil)
		return
	}

	filePaths := []string{
		filepath.Join("/app/output", filename),
		filepath.Join("/app/input", filename),
	}

	var validPath string
	for _, path := range filePaths {
		if fileExists(path) {
			validPath = path
			break
		}
	}

	if validPath == "" {
		gc.respondError(w, http.StatusNotFound, "File not found", nil)
		return
	}

	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", "attachment; filename=\""+filename+"\"")

	http.ServeFile(w, r, validPath)
	log.Printf("INFO: File download completed: %s", filename)
}

func (gc *GDCMController) CleanupFiles(w http.ResponseWriter, r *http.Request) {
	log.Println("INFO: Processing cleanup request")

	var request struct {
		FilePaths []string `json:"file_paths"`
	}
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		log.Printf("ERROR: Failed to decode cleanup request: %v", err)
		gc.respondError(w, http.StatusBadRequest, "Invalid request format", err)
		return
	}

	if len(request.FilePaths) == 0 {
		gc.respondError(w, http.StatusBadRequest, "No file paths provided", nil)
		return
	}

	log.Printf("INFO: Cleaning up %d files", len(request.FilePaths))

	err := gc.gdcmService.CleanupFiles(request.FilePaths...)
	if err != nil {
		log.Printf("ERROR: File cleanup failed: %v", err)
		gc.respondError(w, http.StatusInternalServerError, "File cleanup failed", err)
		return
	}

	log.Printf("INFO: File cleanup completed successfully")
	gc.respondJSON(w, http.StatusOK, map[string]interface{}{
		"message":       "Files cleaned up successfully",
		"files_cleaned": len(request.FilePaths),
	})
}

func (gc *GDCMController) ThumbnailPNG(w http.ResponseWriter, r *http.Request) {
	sid := r.URL.Query().Get("sid")
	if sid == "" {
		gc.respondError(w, http.StatusBadRequest, "Missing sid", nil)
		return
	}
	res, err := gc.gdcmService.ThumbnailPNG(sid)
	if err != nil {
		gc.respondError(w, http.StatusBadGateway, "Failed to fetch thumbnail", err)
		return
	}
	for k, v := range map[string]string{
		"Content-Type":  res.ContentType,
		"Cache-Control": "public, max-age=300",
	} {
		w.Header().Set(k, v)
	}
	w.WriteHeader(res.StatusCode)
	_, _ = w.Write(res.Body)
}

func (gc *GDCMController) RenderFramePNG(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	sid := q.Get("sid")
	if sid == "" {
		gc.respondError(w, http.StatusBadRequest, "Missing sid", nil)
		return
	}
	frameStr := q.Get("frame")
	if frameStr == "" {
		gc.respondError(w, http.StatusBadRequest, "Missing frame", nil)
		return
	}
	frame, err := strconv.Atoi(frameStr)
	if err != nil || frame < 0 {
		gc.respondError(w, http.StatusBadRequest, "Invalid frame", nil)
		return
	}
	var wcPtr, wwPtr *int
	if wcStr := q.Get("wc"); wcStr != "" {
		if v, err := strconv.Atoi(wcStr); err == nil {
			wcPtr = &v
		}
	}
	if wwStr := q.Get("ww"); wwStr != "" {
		if v, err := strconv.Atoi(wwStr); err == nil {
			wwPtr = &v
		}
	}
	res, err := gc.gdcmService.RenderFramePNG(sid, frame, wcPtr, wwPtr)
	if err != nil {
		gc.respondError(w, http.StatusBadGateway, "Failed to render frame", err)
		return
	}
	for k, v := range map[string]string{
		"Content-Type":  res.ContentType,
		"Cache-Control": "public, max-age=60",
	} {
		w.Header().Set(k, v)
	}
	w.WriteHeader(res.StatusCode)
	_, _ = w.Write(res.Body)
}
