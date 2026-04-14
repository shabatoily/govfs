// Package types는 서버 전반에서 사용되는 데이터 구조를 정의합니다.
package types

// CloudListResponse는 클라우드 저장소의 목록 조회 결과를 담고 있는 구조체입니다.
type CloudListResponse struct {
	Path  string   `json:"path"`
	Items []string `json:"items"`
}

// CloudAuthResponse는 클라우드 저장소 인증 프로세스를 위한 URL 정보를 담고 있는 구조체입니다.
type CloudAuthResponse struct {
	URL string `json:"url"`
}
