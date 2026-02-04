package services

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/IntegratedBrainEnvironment/BrainNav_GO_BE/helpers/utils"
)

type DICOMMetadata struct {
	PatientName       string                 `json:"patient_name"`
	PatientID         string                 `json:"patient_id"`
	SeriesDescription string                 `json:"series_description"`
	Modality          string                 `json:"modality"`
	StudyDate         string                 `json:"study_date"`
	SeriesDate        string                 `json:"series_date"`
	StudyDescription  string                 `json:"study_description"`
	SeriesNumber      int                    `json:"series_number"`
	InstanceNumber    int                    `json:"instance_number"`
	SliceThickness    float64                `json:"slice_thickness"`
	Rows              int                    `json:"rows"`
	Columns           int                    `json:"columns"`
	ImagePosition     []float64              `json:"image_position"`
	ImageOrientation  []float64              `json:"image_orientation"`
	PixelSpacing      []float64              `json:"pixel_spacing"`
	SliceLocation     float64                `json:"slice_location"`
	TotalSlices       int                    `json:"total_slices"`
	RawMetadata       map[string]interface{} `json:"raw_metadata,omitempty"`
}

type DICOMSeriesMetadata struct {
	PatientName       string                 `json:"patient_name"`
	PatientID         string                 `json:"patient_id"`
	SeriesDescription string                 `json:"series_description"`
	Modality          string                 `json:"modality"`
	StudyDate         string                 `json:"study_date"`
	SeriesDate        string                 `json:"series_date"`
	StudyDescription  string                 `json:"study_description"`
	SeriesNumber      int                    `json:"series_number"`
	TotalSlices       int                    `json:"total_slices"`
	FirstSlice        *DICOMMetadata         `json:"first_slice,omitempty"`
	MiddleSlice       *DICOMMetadata         `json:"middle_slice,omitempty"`
	LastSlice         *DICOMMetadata         `json:"last_slice,omitempty"`
	SliceThickness    float64                `json:"slice_thickness"`
	Dimensions        string                 `json:"dimensions"`
	RawMetadata       map[string]interface{} `json:"raw_metadata,omitempty"`
}

type DICOMMetadataExtractor struct {
	gdcmBaseURL string
	httpClient  *http.Client
	logger      *utils.Logger
}

func NewDICOMMetadataExtractor(gdcmBaseURL string, logger *utils.Logger) *DICOMMetadataExtractor {
	return &DICOMMetadataExtractor{
		gdcmBaseURL: gdcmBaseURL,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		logger: logger,
	}
}

func (e *DICOMMetadataExtractor) ExtractFromFile(filePath string) (*DICOMMetadata, error) {
	e.logger.Debug("Extracting DICOM metadata from file: %s", filePath)

	if _, err := os.Stat(filePath); err != nil {
		return nil, fmt.Errorf("file not found: %w", err)
	}

	url := e.gdcmBaseURL + "/metadata/extract"

	reqBody := map[string]string{
		"file_path": filePath,
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	resp, err := e.httpClient.Post(url, "application/json", strings.NewReader(string(jsonData)))
	if err != nil {
		return nil, fmt.Errorf("failed to call GDCM service: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GDCM service error (status %d): %s", resp.StatusCode, string(body))
	}

	var result struct {
		Status   string                 `json:"status"`
		Metadata map[string]interface{} `json:"metadata"`
		Error    string                 `json:"error,omitempty"`
	}

	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	if result.Status != "success" {
		return nil, fmt.Errorf("metadata extraction failed: %s", result.Error)
	}

	metadata := &DICOMMetadata{
		RawMetadata: result.Metadata,
	}

	if val, ok := result.Metadata["PatientName"].(string); ok {
		metadata.PatientName = val
	}
	if val, ok := result.Metadata["PatientID"].(string); ok {
		metadata.PatientID = val
	}
	if val, ok := result.Metadata["SeriesDescription"].(string); ok {
		metadata.SeriesDescription = val
	}
	if val, ok := result.Metadata["Modality"].(string); ok {
		metadata.Modality = val
	}
	if val, ok := result.Metadata["StudyDate"].(string); ok {
		metadata.StudyDate = formatDICOMDate(val)
	}
	if val, ok := result.Metadata["SeriesDate"].(string); ok {
		metadata.SeriesDate = formatDICOMDate(val)
	}
	if val, ok := result.Metadata["StudyDescription"].(string); ok {
		metadata.StudyDescription = val
	}
	if val, ok := result.Metadata["SeriesNumber"]; ok {
		metadata.SeriesNumber = parseIntField(val)
	}
	if val, ok := result.Metadata["InstanceNumber"]; ok {
		metadata.InstanceNumber = parseIntField(val)
	}
	if val, ok := result.Metadata["SliceThickness"]; ok {
		metadata.SliceThickness = parseFloatField(val)
	}
	if val, ok := result.Metadata["Rows"]; ok {
		metadata.Rows = parseIntField(val)
	}
	if val, ok := result.Metadata["Columns"]; ok {
		metadata.Columns = parseIntField(val)
	}
	if val, ok := result.Metadata["SliceLocation"]; ok {
		metadata.SliceLocation = parseFloatField(val)
	}

	if val, ok := result.Metadata["ImagePosition"].([]interface{}); ok {
		metadata.ImagePosition = parseFloatArray(val)
	}
	if val, ok := result.Metadata["ImageOrientation"].([]interface{}); ok {
		metadata.ImageOrientation = parseFloatArray(val)
	}
	if val, ok := result.Metadata["PixelSpacing"].([]interface{}); ok {
		metadata.PixelSpacing = parseFloatArray(val)
	}

	e.logger.Debug("Successfully extracted metadata: Patient=%s, Series=%s, Modality=%s",
		metadata.PatientName, metadata.SeriesDescription, metadata.Modality)

	return metadata, nil
}

func (e *DICOMMetadataExtractor) ExtractFromDirectory(dirPath string) (*DICOMSeriesMetadata, error) {
	e.logger.Info("Extracting DICOM metadata from directory: %s", dirPath)

	dicomFiles, err := findDICOMFiles(dirPath)
	if err != nil {
		return nil, fmt.Errorf("failed to find DICOM files: %w", err)
	}

	if len(dicomFiles) == 0 {
		return nil, fmt.Errorf("no DICOM files found in directory: %s", dirPath)
	}

	e.logger.Info("Found %d DICOM files in directory", len(dicomFiles))

	sort.Strings(dicomFiles)

	var firstMetadata, middleMetadata, lastMetadata *DICOMMetadata

	if len(dicomFiles) > 0 {
		firstMetadata, _ = e.ExtractFromFile(dicomFiles[0])
	}

	if len(dicomFiles) > 1 {
		middleIdx := len(dicomFiles) / 2
		middleMetadata, _ = e.ExtractFromFile(dicomFiles[middleIdx])
	}

	if len(dicomFiles) > 2 {
		lastMetadata, _ = e.ExtractFromFile(dicomFiles[len(dicomFiles)-1])
	}

	if firstMetadata == nil {
		return nil, fmt.Errorf("failed to extract metadata from first slice")
	}

	seriesMetadata := &DICOMSeriesMetadata{
		PatientName:       firstMetadata.PatientName,
		PatientID:         firstMetadata.PatientID,
		SeriesDescription: firstMetadata.SeriesDescription,
		Modality:          firstMetadata.Modality,
		StudyDate:         firstMetadata.StudyDate,
		SeriesDate:        firstMetadata.SeriesDate,
		StudyDescription:  firstMetadata.StudyDescription,
		SeriesNumber:      firstMetadata.SeriesNumber,
		TotalSlices:       len(dicomFiles),
		SliceThickness:    firstMetadata.SliceThickness,
		FirstSlice:        firstMetadata,
		MiddleSlice:       middleMetadata,
		LastSlice:         lastMetadata,
	}

	if firstMetadata.Rows > 0 && firstMetadata.Columns > 0 {
		seriesMetadata.Dimensions = fmt.Sprintf("%dx%d", firstMetadata.Rows, firstMetadata.Columns)
	}

	e.logger.Info("Successfully extracted series metadata: Patient=%s, Series=%s, Modality=%s, Slices=%d",
		seriesMetadata.PatientName, seriesMetadata.SeriesDescription, seriesMetadata.Modality, seriesMetadata.TotalSlices)

	return seriesMetadata, nil
}


func findDICOMFiles(dirPath string) ([]string, error) {
	var dicomFiles []string

	err := filepath.Walk(dirPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if info.IsDir() {
			return nil
		}

		ext := strings.ToLower(filepath.Ext(path))
		isDICOM := ext == ".dcm" || ext == ".dicom" || ext == ".dic" || ext == ".dicm" || ext == ""

		if isDICOM {
			dicomFiles = append(dicomFiles, path)
		}

		return nil
	})

	return dicomFiles, err
}

func formatDICOMDate(dateStr string) string {
	if len(dateStr) == 8 {
		year := dateStr[0:4]
		month := dateStr[4:6]
		day := dateStr[6:8]
		return fmt.Sprintf("%s/%s/%s", day, month, year)
	}
	return dateStr
}

func parseIntField(val interface{}) int {
	switch v := val.(type) {
	case int:
		return v
	case int64:
		return int(v)
	case float64:
		return int(v)
	case string:
		if i, err := strconv.Atoi(v); err == nil {
			return i
		}
	}
	return 0
}

func parseFloatField(val interface{}) float64 {
	switch v := val.(type) {
	case float64:
		return v
	case int:
		return float64(v)
	case int64:
		return float64(v)
	case string:
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			return f
		}
	}
	return 0.0
}

func parseFloatArray(arr []interface{}) []float64 {
	result := make([]float64, 0, len(arr))
	for _, val := range arr {
		result = append(result, parseFloatField(val))
	}
	return result
}
