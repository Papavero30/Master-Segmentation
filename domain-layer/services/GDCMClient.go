package services

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/IntegratedBrainEnvironment/BrainNav_GO_BE/helpers/utils"
)

type GDCMClient struct {
	baseURL    string
	httpClient *http.Client
	logger     *utils.Logger
}

func NewGDCMClient(baseURL string, logger *utils.Logger) *GDCMClient {
	return &GDCMClient{
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: 30 * time.Minute,
		},
		logger: logger,
	}
}

type CompressSeriesRequest struct {
	SID         string `json:"sid"`
	InputPath   string `json:"input_path"`
	OutputPath  string `json:"output_path"`
	Compression string `json:"compression"`
}

func (c *GDCMClient) CompressSeries(sid, inputPath, outputPath string) (*SeriesCompressResponse, error) {
	c.logger.Info("Calling GDCM series/compress for SID: %s", sid)
	c.logger.Info("Input path: %s, Output path: %s", inputPath, outputPath)

	reqBody := map[string]string{
		"sid": sid,
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		c.logger.Error("Failed to marshal GDCM request: %v", err)
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	url := c.baseURL + "/series/compress"
	c.logger.Info("POST %s", url)

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		c.logger.Error("Failed to create GDCM request: %v", err)
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		c.logger.Error("GDCM request failed: %v", err)
		return nil, fmt.Errorf("gdcm service unavailable: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		c.logger.Error("Failed to read GDCM response: %v", err)
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	c.logger.Info("GDCM response status: %d", resp.StatusCode)
	c.logger.Info("GDCM response body: %s", string(body))

	if resp.StatusCode != http.StatusOK {
		c.logger.Error("GDCM compression failed with status %d: %s", resp.StatusCode, string(body))
		return nil, fmt.Errorf("gdcm compression failed: %s", string(body))
	}

	var result SeriesCompressResponse
	if err := json.Unmarshal(body, &result); err != nil {
		c.logger.Error("Failed to parse GDCM response: %v", err)
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	if result.Status != "success" && result.Status != "completed" {
		c.logger.Error("GDCM compression reported failure: status=%s", result.Status)
		return nil, fmt.Errorf("compression failed: %s", result.Status)
	}

	c.logger.Info("GDCM compression successful: %d files processed", result.FilesProcessed)
	return &result, nil
}

func (c *GDCMClient) HealthCheck() error {
	url := c.baseURL + "/health"

	resp, err := c.httpClient.Get(url)
	if err != nil {
		return fmt.Errorf("gdcm service unavailable: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("gdcm service unhealthy: status %d", resp.StatusCode)
	}

	c.logger.Info("GDCM service is healthy")
	return nil
}

func (c *GDCMClient) DecompressForRendering(sid, compressedPath, renderPath string) error {
	c.logger.Info("Decompressing DICOM for rendering: %s", sid)

	reqBody := map[string]string{
		"sid":             sid,
		"compressed_path": compressedPath,
		"render_path":     renderPath,
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return fmt.Errorf("failed to marshal request: %w", err)
	}

	url := c.baseURL + "/decompress-for-render"
	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("gdcm service unavailable: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("decompression failed: %s", string(body))
	}

	c.logger.Info("Decompression for rendering successful: %s", sid)
	return nil
}

func (c *GDCMClient) DecompressFile(sid, filename string) ([]byte, error) {
	c.logger.Info("[GDCMClient] Decompressing single file: %s/%s", sid, filename)

	reqBody := map[string]string{
		"sid":      sid,
		"filename": filename,
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		c.logger.Error("[GDCMClient] Failed to marshal decompress request: %v", err)
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	url := c.baseURL + "/file/decompress"
	c.logger.Info("[GDCMClient] POST %s", url)

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		c.logger.Error("[GDCMClient] Failed to create decompress request: %v", err)
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		c.logger.Error("[GDCMClient] Decompress request failed: %v", err)
		return nil, fmt.Errorf("gdcm service unavailable: %w", err)
	}
	defer resp.Body.Close()

	decompressedData, err := io.ReadAll(resp.Body)
	if err != nil {
		c.logger.Error("[GDCMClient] Failed to read decompress response: %v", err)
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	c.logger.Info("[GDCMClient] Response status: %d, size: %d bytes", resp.StatusCode, len(decompressedData))

	if resp.StatusCode != http.StatusOK {
		c.logger.Error("[GDCMClient] Decompression failed with status %d: %s", resp.StatusCode, string(decompressedData))
		return nil, fmt.Errorf("decompression failed: %s", string(decompressedData))
	}

	c.logger.Info("[GDCMClient]  Successfully decompressed %s/%s (%d bytes)", sid, filename, len(decompressedData))
	return decompressedData, nil
}

// FetchSliceData retrieves a specific slice from a DICOM series
func (c *GDCMClient) FetchSliceData(scanSID string, sliceIndex int) ([]byte, error) {
	c.logger.Info("Fetching slice data for SID: %s, slice: %d", scanSID, sliceIndex)

	reqBody := map[string]interface{}{
		"sid":         scanSID,
		"slice_index": sliceIndex,
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		c.logger.Error("Failed to marshal slice fetch request: %v", err)
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	url := c.baseURL + "/slice/fetch"
	c.logger.Info("POST %s", url)

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		c.logger.Error("Failed to create slice fetch request: %v", err)
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		c.logger.Error("Slice fetch request failed: %v", err)
		return nil, fmt.Errorf("gdcm service unavailable: %w", err)
	}
	defer resp.Body.Close()

	sliceData, err := io.ReadAll(resp.Body)
	if err != nil {
		c.logger.Error("Failed to read slice data: %v", err)
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		c.logger.Error("Slice fetch failed with status %d: %s", resp.StatusCode, string(sliceData))
		return nil, fmt.Errorf("slice fetch failed: %s", string(sliceData))
	}

	c.logger.Info("Successfully fetched slice %d for SID %s (%d bytes)", sliceIndex, scanSID, len(sliceData))
	return sliceData, nil
}
