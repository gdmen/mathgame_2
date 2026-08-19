package docs

import "garydmenezes.com/mathgame/server/api"

/*
swagger:route GET /admin/whoami admin adminWhoami
Confirm admin access and report the caller's identity and role.
responses:
  200: adminWhoamiResp
  403: error
*/

/*
swagger:route GET /admin/difficulty-calibration admin adminDifficultyCalibration
Get the cached difficulty-calibration report.
report is null until one has been computed; computing says whether a rebuild is
in flight.
responses:
  200: calibrationResp
  403: error
  500: error
*/

/*
swagger:route POST /admin/difficulty-calibration/recompute admin adminRecomputeCalibration
Start a background rebuild of the calibration report.
Returns immediately. A second request while one is already running is a no-op.
responses:
  200: computingResp
  403: error
*/

/*
swagger:route GET /admin/bitmap-matrix admin adminBitmapMatrix
Get the cached bitmap-matrix report.
The body is the raw gzipped report (Content-Encoding: gzip) and is empty until
one has been computed. The envelope rides in response headers instead:
X-Has-Report, X-Computing, X-Compute-Done, X-Compute-Total, X-Computed-At.
responses:
  200: bitmapMatrixResp
  403: error
  500: error
*/

/*
swagger:route POST /admin/bitmap-matrix/recompute admin adminRecomputeBitmapMatrix
Start a background rebuild of the bitmap-matrix report.
Returns immediately. A second request while one is already running is a no-op.
responses:
  200: computingResp
  403: error
*/

/*
swagger:route GET /admin/bitmap-matrix/cell admin adminBitmapMatrixCell
Live-regenerate one bitmap-matrix cell.
Ephemeral: the cached report is never touched. 400 when the bitmap is invalid
or the bucket is not targetable for it; 422 when no problem could be built.
responses:
  200: matrixCellResp
  400: error
  403: error
  422: error
*/

/*
swagger:route GET /admin/swagger.yaml admin adminSwaggerSpec
Get this spec, as the admin API docs page loads it.
produces:
- application/yaml
responses:
  200: swaggerSpecResp
  403: error
*/

// swagger:parameters adminBitmapMatrixCell
type adminBitmapMatrixCellParameters struct {
	// in:query
	// required: true
	Bitmap uint64 `json:"bitmap"`
	// Difficulty bucket, which must be targetable for this bitmap.
	// in:query
	// required: true
	Bucket int `json:"bucket"`
}

// swagger:response adminWhoamiResp
type adminWhoamiResponse struct {
	// in:body
	Body struct {
		Auth0Id string `json:"auth0_id"`
		Id      uint32 `json:"id"`
		Role    string `json:"role"`
	}
}

// swagger:response calibrationResp
type calibrationResponse struct {
	// in:body
	Body api.CalibrationReportResponse
}

// swagger:response computingResp
type computingResponse struct {
	// in:body
	Body struct {
		Computing bool `json:"computing"`
	}
}

// swagger:response bitmapMatrixResp
type bitmapMatrixResponse struct {
	// Whether a cached report exists.
	XHasReport bool `json:"X-Has-Report"`
	// Whether a rebuild is in flight.
	XComputing bool `json:"X-Computing"`
	// Bitmaps processed so far by the in-flight rebuild.
	XComputeDone int64 `json:"X-Compute-Done"`
	// Bitmaps the in-flight rebuild will process.
	XComputeTotal int64 `json:"X-Compute-Total"`
	// When the cached report was computed.
	XComputedAt string `json:"X-Computed-At"`
	// in:body
	Body api.BitmapMatrixData
}

// swagger:response matrixCellResp
type matrixCellResponse struct {
	// in:body
	Body api.MatrixCell
}
