package controller

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/IntegratedBrainEnvironment/BrainNav_GO_BE/domain-layer/middleware"
	"github.com/IntegratedBrainEnvironment/BrainNav_GO_BE/domain-layer/services"
	"github.com/IntegratedBrainEnvironment/BrainNav_GO_BE/helpers/dto"
	"github.com/IntegratedBrainEnvironment/BrainNav_GO_BE/helpers/utils"
	"github.com/gorilla/mux"
)

type ScansController struct {
	BaseController
	scansService services.ScansService
}

func NewScansController(scansService services.ScansService) *ScansController {
	return &ScansController{
		BaseController: BaseController{},
		scansService:   scansService,
	}
}

func (c *ScansController) GetAllScans(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)

	profileFirstUid := vars["profileFirstUid"]
	if profileFirstUid == "" {
		profileFirstUid = vars["patientID"]
	}

	if profileFirstUid == "" {
		c.RespondWithError(w, utils.NewBadRequestError("Invalid profile FirstUid or patient ID"))
		return
	}

	scans, err := c.scansService.GetAllScans(r.Context(), profileFirstUid)
	if err != nil {
		c.RespondWithError(w, err)
		return
	}

	c.RespondWithJSON(w, http.StatusOK, scans)
}

func (c *ScansController) GetAllScansQuery(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	profileFirstUid := q.Get("profile_first_uid")
	if profileFirstUid == "" {
		profileFirstUid = q.Get("patient_id")
	}
	if profileFirstUid == "" {
		c.RespondWithError(w, utils.NewBadRequestError("profile_first_uid or patient_id query required"))
		return
	}
	scans, err := c.scansService.GetAllScans(r.Context(), profileFirstUid)
	if err != nil {
		c.RespondWithError(w, err)
		return
	}
	c.RespondWithJSON(w, http.StatusOK, scans)
}

func (c *ScansController) GetScanBySid(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	sid := vars["sid"]
	if sid == "" {
		c.RespondWithError(w, utils.NewBadRequestError("Invalid scan SID"))
		return
	}

	scan, err := c.scansService.GetScanBySid(r.Context(), sid)
	if err != nil {
		c.RespondWithError(w, err)
		return
	}
	device, ok := middleware.GetDeviceFromContext(r.Context())
	if ok && scan.OwnerDeviceID != device.ID {
		c.RespondWithError(w, utils.NewNotFoundError("Scan", sid))
		return
	}

	c.RespondWithJSON(w, http.StatusOK, scan)
}

func (c *ScansController) GetScanByID(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id, err := strconv.Atoi(vars["id"])
	if err != nil {
		c.RespondWithError(w, utils.NewBadRequestError("Invalid scan ID"))
		return
	}

	scan, err := c.scansService.GetScanByID(r.Context(), id)
	if err != nil {
		c.RespondWithError(w, err)
		return
	}
	device, ok := middleware.GetDeviceFromContext(r.Context())
	if ok && scan.OwnerDeviceID != device.ID {
		c.RespondWithError(w, utils.NewNotFoundError("Scan", id))
		return
	}

	c.RespondWithJSON(w, http.StatusOK, scan)
}

func (c *ScansController) CreateScan(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	profileFirstUid := vars["profileFirstUid"]
	if profileFirstUid == "" {
		c.RespondWithError(w, utils.NewBadRequestError("Invalid profile FirstUid"))
		return
	}

	var request dto.ScansCreateRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		c.RespondWithError(w, utils.NewBadRequestError("Invalid request payload"))
		return
	}
	defer r.Body.Close()

	request.ProfileFirstUid = profileFirstUid

	scan, err := c.scansService.CreateScan(r.Context(), &request)
	if err != nil {
		c.RespondWithError(w, err)
		return
	}

	c.RespondWithJSON(w, http.StatusCreated, scan)
}

func (c *ScansController) UpdateScan(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	sid := vars["sid"]
	if sid == "" {
		c.RespondWithError(w, utils.NewBadRequestError("Invalid scan SID"))
		return
	}

	var request dto.ScansUpdateRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		c.RespondWithError(w, utils.NewBadRequestError("Invalid request payload"))
		return
	}
	defer r.Body.Close()

	scan, err := c.scansService.UpdateScan(r.Context(), sid, &request)
	if err != nil {
		c.RespondWithError(w, err)
		return
	}
	device, ok := middleware.GetDeviceFromContext(r.Context())
	if ok && scan.OwnerDeviceID != device.ID {
		c.RespondWithError(w, utils.NewNotFoundError("Scan", sid))
		return
	}

	c.RespondWithJSON(w, http.StatusOK, scan)
}

func (c *ScansController) DeleteScan(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	sid := vars["sid"]
	if sid == "" {
		c.RespondWithError(w, utils.NewBadRequestError("Invalid scan SID"))
		return
	}

	if err := c.scansService.DeleteScan(r.Context(), sid); err != nil {
		c.RespondWithError(w, err)
		return
	}

	c.RespondWithJSON(w, http.StatusOK, map[string]string{"message": "Scan deleted successfully"})
}

type InitUploadRequest struct {
	ProfileFirstUid   string   `json:"profile_first_uid"`
	ExpectedFileCount int      `json:"expected_file_count"`
	FileNames         []string `json:"file_names"`
	PatientName       string   `json:"patient_name"`
}

func (c *ScansController) InitUpload(w http.ResponseWriter, r *http.Request) {
	var payload InitUploadRequest
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		c.RespondWithError(w, utils.NewBadRequestError("invalid payload"))
		return
	}
	vars := mux.Vars(r)
	sid := vars["sid"]
	if sid == "" {
		c.RespondWithError(w, utils.NewBadRequestError("sid required"))
		return
	}
	resp, err := c.scansService.InitUpload(r.Context(), sid, payload.ProfileFirstUid, payload.ExpectedFileCount, payload.FileNames, payload.PatientName)
	if err != nil {
		c.RespondWithError(w, err)
		return
	}
	c.RespondWithJSON(w, http.StatusOK, resp)
}

func (c *ScansController) GetStatus(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	sid := vars["sid"]
	if sid == "" {
		c.RespondWithError(w, utils.NewBadRequestError("sid required"))
		return
	}
	resp, err := c.scansService.GetStatus(r.Context(), sid)
	if err != nil {
		c.RespondWithError(w, err)
		return
	}
	c.RespondWithJSON(w, http.StatusOK, resp)
}

func (c *ScansController) UploadFile(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	sid := vars["sid"]
	if sid == "" {
		c.RespondWithError(w, utils.NewBadRequestError("sid required"))
		return
	}
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		c.RespondWithError(w, utils.NewBadRequestError("invalid multipart"))
		return
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		c.RespondWithError(w, utils.NewBadRequestError("file required"))
		return
	}
	defer file.Close()
	relPath := r.FormValue("rel_path")
	if relPath == "" {
		relPath = header.Filename
	}
	if err := c.scansService.UploadFileChunk(r.Context(), sid, relPath, file, header.Size); err != nil {
		c.RespondWithError(w, err)
		return
	}
	c.RespondWithJSON(w, http.StatusOK, map[string]string{"message": "uploaded", "rel_path": relPath})
}

// UploadZip handles bulk upload of DICOM files as a ZIP archive
// This is much faster than uploading files one by one
func (c *ScansController) UploadZip(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	sid := vars["sid"]
	if sid == "" {
		c.RespondWithError(w, utils.NewBadRequestError("sid required"))
		return
	}

	// Parse multipart form with large max memory (512MB for ZIP files)
	if err := r.ParseMultipartForm(512 << 20); err != nil {
		c.RespondWithError(w, utils.NewBadRequestError("invalid multipart form: "+err.Error()))
		return
	}

	file, header, err := r.FormFile("zip_file")
	if err != nil {
		c.RespondWithError(w, utils.NewBadRequestError("zip_file required"))
		return
	}
	defer file.Close()

	// Call service to handle ZIP extraction
	result, err := c.scansService.UploadZip(r.Context(), sid, file, header.Size)
	if err != nil {
		c.RespondWithError(w, err)
		return
	}

	c.RespondWithJSON(w, http.StatusOK, result)
}

// UploadBatch handles batch upload of multiple DICOM files in a single request
// This reduces HTTP overhead compared to uploading files one by one
func (c *ScansController) UploadBatch(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	sid := vars["sid"]
	if sid == "" {
		c.RespondWithError(w, utils.NewBadRequestError("sid required"))
		return
	}

	// Parse multipart form with large max memory (256MB for batch files)
	if err := r.ParseMultipartForm(256 << 20); err != nil {
		c.RespondWithError(w, utils.NewBadRequestError("invalid multipart form: "+err.Error()))
		return
	}

	// Get all files from the "files" field
	files := r.MultipartForm.File["files"]
	if len(files) == 0 {
		c.RespondWithError(w, utils.NewBadRequestError("no files provided"))
		return
	}

	result, err := c.scansService.UploadBatch(r.Context(), sid, files)
	if err != nil {
		c.RespondWithError(w, err)
		return
	}

	c.RespondWithJSON(w, http.StatusOK, result)
}

func (c *ScansController) CompleteUpload(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	sid := vars["sid"]
	force := r.URL.Query().Get("force") == "1"
	resp, err := c.scansService.CompleteUpload(r.Context(), sid, force)
	if err != nil {
		c.RespondWithError(w, err)
		return
	}

	c.RespondWithJSON(w, http.StatusAccepted, map[string]interface{}{
		"message": "Upload completed. Compression is processing in background.",
		"scan":    resp,
		"status":  resp.UploadStatus,
		"sid":     sid,
	})
}

func (c *ScansController) AbortUpload(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	sid := vars["sid"]
	if err := c.scansService.AbortUpload(r.Context(), sid); err != nil {
		c.RespondWithError(w, err)
		return
	}
	c.RespondWithJSON(w, http.StatusOK, map[string]string{"message": "aborted"})
}

func (c *ScansController) ListFiles(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	sid := vars["sid"]
	if sid == "" {
		c.RespondWithError(w, utils.NewBadRequestError("sid required"))
		return
	}
	resp, err := c.scansService.ListFiles(r.Context(), sid)
	if err != nil {
		c.RespondWithError(w, err)
		return
	}
	c.RespondWithJSON(w, http.StatusOK, resp)
}

func (c *ScansController) DownloadFile(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	sid := vars["sid"]
	if sid == "" {
		c.RespondWithError(w, utils.NewBadRequestError("sid required"))
		return
	}
	relPath := vars["path"]
	if relPath == "" {
		relPath = r.URL.Query().Get("path")
	}
	abs, sf, err := c.scansService.ResolveFilePath(r.Context(), sid, relPath)
	if err != nil {
		c.RespondWithError(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/dicom")
	w.Header().Set("X-File-Hash", sf.Hash)
	w.Header().Set("X-File-Size", strconv.FormatInt(sf.Size, 10))
	w.Header().Set("X-Content-Type-Options", "nosniff")
	http.ServeFile(w, r, abs)
}

func (c *ScansController) DecompressFile(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	sid := vars["sid"]
	filename := vars["filename"]

	if sid == "" || filename == "" {
		c.RespondWithError(w, utils.NewBadRequestError("sid and filename are required"))
		return
	}

	decompressedData, err := c.scansService.DecompressFile(r.Context(), sid, filename)
	if err != nil {
		c.RespondWithError(w, err)
		return
	}

	w.Header().Set("Content-Type", "application/dicom")
	w.Header().Set("Content-Length", strconv.Itoa(len(decompressedData)))
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("X-Decompressed", "true")
	w.Header().Set("Cache-Control", "public, max-age=86400")

	if _, err := w.Write(decompressedData); err != nil {
		c.RespondWithError(w, utils.NewInternalServerError("Failed to write response", err))
		return
	}
}

func (c *ScansController) DecompressBatch(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	sid := vars["sid"]

	if sid == "" {
		c.RespondWithError(w, utils.NewBadRequestError("sid is required"))
		return
	}

	var req struct {
		Filenames []string `json:"filenames"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		c.RespondWithError(w, utils.NewBadRequestError("Invalid request body"))
		return
	}

	if len(req.Filenames) == 0 {
		c.RespondWithError(w, utils.NewBadRequestError("filenames array is required"))
		return
	}

	result, err := c.scansService.DecompressBatch(r.Context(), sid, req.Filenames)
	if err != nil {
		c.RespondWithError(w, err)
		return
	}

	fileUrls := make(map[string]string)
	for filename := range result {
		fileUrls[filename] = "/api/scans/" + sid + "/decompress/" + filename
	}

	c.RespondWithJSON(w, http.StatusOK, map[string]interface{}{
		"status":     "success",
		"sid":        sid,
		"file_count": len(result),
		"file_urls":  fileUrls,
	})
}

func (c *ScansController) GetCacheStats(w http.ResponseWriter, r *http.Request) {
	stats := c.scansService.GetCacheStats(r.Context())
	c.RespondWithJSON(w, http.StatusOK, stats)
}

func (c *ScansController) ClearScanCache(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	sid := vars["sid"]

	if sid == "" {
		c.RespondWithError(w, utils.NewBadRequestError("sid is required"))
		return
	}

	if err := c.scansService.ClearScanCache(r.Context(), sid); err != nil {
		c.RespondWithError(w, err)
		return
	}

	c.RespondWithJSON(w, http.StatusOK, map[string]interface{}{
		"status":  "success",
		"message": "Cache cleared for scan: " + sid,
	})
}
