package services

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

type GDCMService struct {
	baseURL    string
	httpClient *http.Client
}

func (g *GDCMService) BaseURL() string { return g.baseURL }

func (g *GDCMService) HTTP() *http.Client { return g.httpClient }

type GDCMCompressRequest struct {
	InputPath  string `json:"input_path"`
	OutputPath string `json:"output_path"`
}

type GDCMDecompressRequest struct {
	InputPath  string `json:"input_path"`
	OutputPath string `json:"output_path"`
}

type GDCMInfoRequest struct {
	FilePath string `json:"file_path"`
}

type GDCMBatchCompressRequest struct {
	Files []struct {
		Input  string `json:"input"`
		Output string `json:"output"`
	} `json:"files"`
}

type GDCMResponse struct {
	Status                  string                 `json:"status"`
	Message                 string                 `json:"message,omitempty"`
	OriginalSizeBytes       int64                  `json:"original_size_bytes,omitempty"`
	CompressedSizeBytes     int64                  `json:"compressed_size_bytes,omitempty"`
	CompressionRatioPercent float64                `json:"compression_ratio_percent,omitempty"`
	TransferSyntax          string                 `json:"transfer_syntax,omitempty"`
	ProcessedAt             string                 `json:"processed_at,omitempty"`
	Info                    map[string]interface{} `json:"info,omitempty"`
}

type GDCMBatchResponse struct {
	Status  string `json:"status"`
	Summary struct {
		TotalFiles int `json:"total_files"`
		Successful int `json:"successful"`
		Failed     int `json:"failed"`
	} `json:"summary"`
	Results []GDCMResponse `json:"results"`
}

type GDCMHealthResponse struct {
	Status      string `json:"status"`
	Service     string `json:"service"`
	Version     string `json:"version"`
	GDCMVersion string `json:"gdcm_version"`
	Timestamp   string `json:"timestamp"`
}

type SeriesManifestFile struct {
	Path           string  `json:"path"`
	Size           int64   `json:"size"`
	Hash           string  `json:"hash"`
	InstanceNumber int     `json:"instance_number"`
	SOPInstanceUID string  `json:"sop_instance_uid"`
	SliceLocation  float64 `json:"slice_location,omitempty"`
}

type SeriesManifestGroup struct {
	GroupID    string               `json:"group_id"`
	GroupIndex int                  `json:"group_index"`
	SliceCount int                  `json:"slice_count"`
	Files      []SeriesManifestFile `json:"files"`
	Stats      SeriesManifestStats  `json:"stats"`
}

type SeriesManifestStats struct {
	OriginalSizeBytes       int64   `json:"original_size_bytes"`
	CompressedSizeBytes     int64   `json:"compressed_size_bytes"`
	CompressionRatioPercent float64 `json:"compression_ratio_percent"`
	FilesProcessed          int     `json:"files_processed"`
	FilesFailed             int     `json:"files_failed"`
}

type SeriesManifest struct {
	Sid            string                `json:"sid"`
	GeneratedAt    string                `json:"generated_at"`
	TransferSyntax string                `json:"transfer_syntax"`
	TotalGroups    int                   `json:"total_groups,omitempty"`
	Groups         []SeriesManifestGroup `json:"groups,omitempty"`
	Files          []SeriesManifestFile  `json:"files"`
	Stats          SeriesManifestStats   `json:"stats"`
}

type SeriesCompressResponse struct {
	Status                  string         `json:"status"`
	Sid                     string         `json:"sid"`
	Operation               string         `json:"operation"`
	TotalGroups             int            `json:"total_groups,omitempty"`
	FilesProcessed          int            `json:"files_processed"`
	FilesFailed             int            `json:"files_failed"`
	OriginalSizeBytes       int64          `json:"original_size_bytes"`
	CompressedSizeBytes     int64          `json:"compressed_size_bytes"`
	CompressionRatioPercent float64        `json:"compression_ratio_percent"`
	TransferSyntax          string         `json:"transfer_syntax"`
	ManifestPath            string         `json:"manifest_path"`
	Manifest                SeriesManifest `json:"manifest"`
}

type SeriesManifestResponse struct {
	Status   string         `json:"status"`
	Sid      string         `json:"sid"`
	Manifest SeriesManifest `json:"manifest"`
}

type SeriesStatsResponse struct {
	Sid                string      `json:"sid"`
	RawExists          bool        `json:"raw_exists"`
	CompressedExists   bool        `json:"compressed_exists"`
	DecompressedExists bool        `json:"decompressed_exists"`
	Status             string      `json:"status"`
	ManifestPath       string      `json:"manifest_path,omitempty"`
	Stats              interface{} `json:"stats,omitempty"`
}

type SeriesDecompressResponse struct {
	Status         string `json:"status"`
	Sid            string `json:"sid"`
	Operation      string `json:"operation"`
	FilesProcessed int    `json:"files_processed"`
	FilesFailed    int    `json:"files_failed"`
}

type FileDecompressRequest struct {
	Sid      string `json:"sid"`
	FilePath string `json:"file_path"`
}

type FileDecompressResponse struct {
	Status   string `json:"status"`
	FilePath string `json:"file_path"`
	Message  string `json:"message,omitempty"`
	Error    string `json:"error,omitempty"`
	Details  string `json:"details,omitempty"`
}

func NewGDCMService() *GDCMService {
	gdcmURL := os.Getenv("GDCM_SERVICE_URL")
	if gdcmURL == "" {
		gdcmURL = "http://brainnav-gdcm:3000"
	}

	return &GDCMService{
		baseURL: gdcmURL,
		httpClient: &http.Client{
			Timeout: 5 * time.Minute,
		},
	}
}

type PNGFetchResult struct {
	StatusCode  int
	ContentType string
	Body        []byte
}

func (g *GDCMService) fetchPNG(endpoint string, query map[string]string) (*PNGFetchResult, error) {
	req, err := http.NewRequest("GET", g.baseURL+endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	q := req.URL.Query()
	for k, v := range query {
		q.Set(k, v)
	}
	req.URL.RawQuery = q.Encode()
	req.Header.Set("Accept", "image/png")

	resp, err := g.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read image body: %w", err)
	}
	ct := resp.Header.Get("Content-Type")
	if ct == "" {
		ct = "image/png"
	}
	return &PNGFetchResult{StatusCode: resp.StatusCode, ContentType: ct, Body: body}, nil
}

func (g *GDCMService) ThumbnailPNG(sid string) (*PNGFetchResult, error) {
	if sid == "" {
		return nil, fmt.Errorf("sid is required")
	}
	return g.fetchPNG("/thumbnail", map[string]string{"sid": sid})
}

func (g *GDCMService) RenderFramePNG(sid string, frame int, wc, ww *int) (*PNGFetchResult, error) {
	if sid == "" {
		return nil, fmt.Errorf("sid is required")
	}
	q := map[string]string{
		"sid":   sid,
		"frame": fmt.Sprintf("%d", frame),
	}
	if wc != nil {
		q["wc"] = fmt.Sprintf("%d", *wc)
	}
	if ww != nil {
		q["ww"] = fmt.Sprintf("%d", *ww)
	}
	return g.fetchPNG("/render-frame", q)
}

func (g *GDCMService) HealthCheck() (*GDCMHealthResponse, error) {
	log.Println("INFO: Checking GDCM service health")

	resp, err := g.httpClient.Get(g.baseURL + "/health")
	if err != nil {
		log.Printf("ERROR: GDCM health check failed - URL: %s, Error: %v", g.baseURL+"/health", err)
		return nil, fmt.Errorf("failed to connect to GDCM service: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		log.Printf("ERROR: GDCM health check returned non-200 status - Status: %d, URL: %s", resp.StatusCode, g.baseURL+"/health")
		return nil, fmt.Errorf("GDCM service returned status: %d", resp.StatusCode)
	}

	var healthResponse GDCMHealthResponse
	if err := json.NewDecoder(resp.Body).Decode(&healthResponse); err != nil {
		log.Printf("ERROR: Failed to decode GDCM health response - Error: %v", err)
		return nil, fmt.Errorf("failed to decode health response: %w", err)
	}

	log.Printf("INFO: GDCM service is healthy - Service: %s, Version: %s, GDCM Version: %s",
		healthResponse.Service, healthResponse.Version, healthResponse.GDCMVersion)

	return &healthResponse, nil
}

func (g *GDCMService) SeriesCompress(sid string) (*SeriesCompressResponse, error) {
	sid = strings.TrimSpace(sid)
	if sid == "" {
		return nil, fmt.Errorf("sid is required")
	}
	body, err := json.Marshal(map[string]string{"sid": sid})
	if err != nil {
		return nil, fmt.Errorf("failed to encode request: %w", err)
	}
	req, err := http.NewRequest(http.MethodPost, g.baseURL+"/series/compress", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := g.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("series compress request failed: %w", err)
	}
	defer resp.Body.Close()
	var parsed SeriesCompressResponse
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return nil, fmt.Errorf("failed to decode series compress response: %w", err)
	}
	if resp.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("raw series not found")
	}
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("gdcm series compress returned status %d", resp.StatusCode)
	}
	if !strings.EqualFold(parsed.Status, "success") {
		return &parsed, fmt.Errorf("gdcm series compress returned status %s", parsed.Status)
	}
	return &parsed, nil
}

func (g *GDCMService) SeriesDecompress(sid string) (*SeriesDecompressResponse, error) {
	sid = strings.TrimSpace(sid)
	if sid == "" {
		return nil, fmt.Errorf("sid is required")
	}
	body, err := json.Marshal(map[string]string{"sid": sid})
	if err != nil {
		return nil, fmt.Errorf("failed to encode request: %w", err)
	}
	req, err := http.NewRequest(http.MethodPost, g.baseURL+"/series/decompress", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := g.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("series decompress request failed: %w", err)
	}
	defer resp.Body.Close()

	var parsed SeriesDecompressResponse
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return nil, fmt.Errorf("failed to decode series decompress response: %w", err)
	}
	if resp.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("compressed series not found")
	}
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("gdcm series decompress returned status %d", resp.StatusCode)
	}
	status := strings.ToLower(parsed.Status)
	if status != "success" && status != "cached" {
		return &parsed, fmt.Errorf("gdcm series decompress returned status %s", parsed.Status)
	}
	return &parsed, nil
}

func (g *GDCMService) DecompressFile(sid, filePath string) (*FileDecompressResponse, error) {
	sid = strings.TrimSpace(sid)
	filePath = strings.TrimSpace(filePath)
	if sid == "" {
		return nil, fmt.Errorf("sid is required")
	}
	if filePath == "" {
		return nil, fmt.Errorf("file_path is required")
	}

	reqBody := FileDecompressRequest{
		Sid:      sid,
		FilePath: filePath,
	}
	body, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to encode request: %w", err)
	}

	req, err := http.NewRequest(http.MethodPost, g.baseURL+"/decompress/file", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := g.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("file decompress request failed: %w", err)
	}
	defer resp.Body.Close()

	var parsed FileDecompressResponse
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return nil, fmt.Errorf("failed to decode file decompress response: %w", err)
	}

	if resp.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("source file not found")
	}
	if resp.StatusCode >= 400 {
		if parsed.Error != "" {
			return nil, fmt.Errorf("gdcm decompress file failed: %s", parsed.Error)
		}
		return nil, fmt.Errorf("gdcm decompress file returned status %d", resp.StatusCode)
	}

	status := strings.ToLower(parsed.Status)
	if status != "success" && status != "cached" && status != "skipped" {
		return &parsed, fmt.Errorf("gdcm decompress file returned status %s", parsed.Status)
	}

	return &parsed, nil
}

func (g *GDCMService) FetchSeriesManifest(sid string) (*SeriesManifestResponse, error) {
	sid = strings.TrimSpace(sid)
	if sid == "" {
		return nil, fmt.Errorf("sid is required")
	}
	req, err := http.NewRequest(http.MethodGet, g.baseURL+"/series/manifest", nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create manifest request: %w", err)
	}
	q := req.URL.Query()
	q.Set("sid", sid)
	req.URL.RawQuery = q.Encode()
	resp, err := g.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("manifest request failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("manifest not found")
	}
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("gdcm manifest request status %d", resp.StatusCode)
	}
	var parsed SeriesManifestResponse
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return nil, fmt.Errorf("failed to decode manifest response: %w", err)
	}
	return &parsed, nil
}

func (g *GDCMService) SeriesStats(sid string) (*SeriesStatsResponse, error) {
	sid = strings.TrimSpace(sid)
	if sid == "" {
		return nil, fmt.Errorf("sid is required")
	}
	req, err := http.NewRequest(http.MethodGet, g.baseURL+"/series/stats", nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create stats request: %w", err)
	}
	q := req.URL.Query()
	q.Set("sid", sid)
	req.URL.RawQuery = q.Encode()
	resp, err := g.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("series stats request failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("gdcm series stats status %d", resp.StatusCode)
	}
	var parsed SeriesStatsResponse
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return nil, fmt.Errorf("failed to decode series stats response: %w", err)
	}
	return &parsed, nil
}

func (g *GDCMService) CompressDICOM(inputPath, outputPath string) (*GDCMResponse, error) {
	log.Printf("INFO: Starting DICOM compression - Input: %s, Output: %s", inputPath, outputPath)

	request := GDCMCompressRequest{
		InputPath:  inputPath,
		OutputPath: outputPath,
	}

	response, err := g.makeRequest("POST", "/compress", request)
	if err != nil {
		log.Printf("ERROR: DICOM compression failed - Input: %s, Output: %s, Error: %v", inputPath, outputPath, err)
		return nil, err
	}

	log.Printf("INFO: DICOM compression completed - Status: %s, Compression: %.2f%%, Original: %d bytes, Compressed: %d bytes",
		response.Status, response.CompressionRatioPercent, response.OriginalSizeBytes, response.CompressedSizeBytes)

	return response, nil
}

func (g *GDCMService) DecompressDICOM(inputPath, outputPath string) (*GDCMResponse, error) {
	log.Printf("INFO: Starting DICOM decompression - Input: %s, Output: %s", inputPath, outputPath)

	request := GDCMDecompressRequest{
		InputPath:  inputPath,
		OutputPath: outputPath,
	}

	response, err := g.makeRequest("POST", "/decompress", request)
	if err != nil {
		log.Printf("ERROR: DICOM decompression failed - Input: %s, Output: %s, Error: %v", inputPath, outputPath, err)
		return nil, err
	}

	log.Printf("INFO: DICOM decompression completed - Status: %s", response.Status)

	return response, nil
}

func (g *GDCMService) GetDICOMInfo(filePath string) (*GDCMResponse, error) {
	log.Printf("INFO: Getting DICOM info - File: %s", filePath)

	request := GDCMInfoRequest{
		FilePath: filePath,
	}

	response, err := g.makeRequest("POST", "/info", request)
	if err != nil {
		log.Printf("ERROR: DICOM info extraction failed - File: %s, Error: %v", filePath, err)
		return nil, err
	}

	log.Printf("INFO: DICOM info extracted successfully - Status: %s, File: %s", response.Status, filePath)

	return response, nil
}

func (g *GDCMService) BatchCompressDICOM(files []struct {
	Input  string
	Output string
}) (*GDCMBatchResponse, error) {
	log.Printf("INFO: Starting batch DICOM compression - File count: %d", len(files))

	request := GDCMBatchCompressRequest{
		Files: make([]struct {
			Input  string `json:"input"`
			Output string `json:"output"`
		}, len(files)),
	}

	for i, file := range files {
		request.Files[i].Input = file.Input
		request.Files[i].Output = file.Output
	}

	jsonData, err := json.Marshal(request)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal batch request: %w", err)
	}

	resp, err := g.httpClient.Post(
		g.baseURL+"/batch-compress",
		"application/json",
		bytes.NewBuffer(jsonData),
	)
	if err != nil {
		log.Printf("ERROR: Batch compression request failed - Error: %v", err)
		return nil, fmt.Errorf("failed to send batch compression request: %w", err)
	}
	defer resp.Body.Close()

	var batchResponse GDCMBatchResponse
	if err := json.NewDecoder(resp.Body).Decode(&batchResponse); err != nil {
		return nil, fmt.Errorf("failed to decode batch response: %w", err)
	}

	log.Printf("INFO: Batch DICOM compression completed - Status: %s, Total: %d, Successful: %d, Failed: %d",
		batchResponse.Status, batchResponse.Summary.TotalFiles, batchResponse.Summary.Successful, batchResponse.Summary.Failed)

	return &batchResponse, nil
}

func (g *GDCMService) UploadAndProcessDICOM(file multipart.File, filename string, compress bool, sid string, final bool) (*GDCMResponse, error) {
	start := time.Now()
	log.Printf("INFO: Proxy uploading DICOM - filename=%s sid=%s compress=%t final=%t", filename, sid, compress, final)
	if filepath.Base(filename) != filename {
		return nil, fmt.Errorf("invalid filename path traversal")
	}
	if matched, _ := regexp.MatchString(`(?i)[<>:"\\|?*]`, filename); matched {
		return nil, fmt.Errorf("invalid characters in filename")
	}
	const maxSize = 200 << 20
	var buf bytes.Buffer
	if _, err := io.Copy(&buf, io.LimitReader(file, maxSize+1)); err != nil {
		return nil, fmt.Errorf("failed to read upload stream: %w", err)
	}
	if int64(buf.Len()) > maxSize {
		return nil, fmt.Errorf("file exceeds max size %d bytes", maxSize)
	}
	body := &bytes.Buffer{}
	mw := multipart.NewWriter(body)
	fw, err := mw.CreateFormFile("dicom_file", filename)
	if err != nil {
		return nil, fmt.Errorf("failed to create form file: %w", err)
	}
	if _, err = io.Copy(fw, bytes.NewReader(buf.Bytes())); err != nil {
		return nil, fmt.Errorf("failed to copy file data: %w", err)
	}
	if sid != "" {
		_ = mw.WriteField("sid", sid)
	}
	_ = mw.WriteField("compress", fmt.Sprintf("%t", compress))
	_ = mw.WriteField("final", fmt.Sprintf("%t", final))
	if err = mw.Close(); err != nil {
		return nil, fmt.Errorf("failed to close multipart writer: %w", err)
	}
	endpoint := g.baseURL + "/upload"
	req, err := http.NewRequest("POST", endpoint, body)
	if err != nil {
		return nil, fmt.Errorf("failed to create upload request: %w", err)
	}
	req.Header.Set("Content-Type", mw.FormDataContentType())
	resp, err := g.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("upload request failed: %w", err)
	}
	defer resp.Body.Close()
	rb, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("upload failed status=%d body=%s", resp.StatusCode, string(rb))
	}
	var parsed map[string]interface{}
	_ = json.Unmarshal(rb, &parsed)
	log.Printf("INFO: Upload complete sid=%s filename=%s bytes=%d dur_ms=%d", sid, filename, buf.Len(), time.Since(start).Milliseconds())
	return &GDCMResponse{Status: "success", Message: "stored"}, nil
}

func (g *GDCMService) CleanupFiles(filePaths ...string) error {
	for _, path := range filePaths {
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			log.Printf("ERROR: Failed to cleanup file - Path: %s, Error: %v", path, err)
			return fmt.Errorf("failed to remove file %s: %w", path, err)
		}
	}
	return nil
}

func (g *GDCMService) makeRequest(method, endpoint string, data interface{}) (*GDCMResponse, error) {
	jsonData, err := json.Marshal(data)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request data: %w", err)
	}

	req, err := http.NewRequest(method, g.baseURL+endpoint, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := g.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	var response GDCMResponse
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	if resp.StatusCode != http.StatusOK || response.Status != "success" {
		return &response, fmt.Errorf("GDCM service error: %s", response.Message)
	}

	return &response, nil
}
