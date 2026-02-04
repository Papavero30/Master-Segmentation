package dto


type ProfilesResponse struct {
	ID       uint   `json:"id"`
	FirstUid string `json:"first_uid"`
	Name     string `json:"name"`
}


type ProfilesCreateRequest struct {
	FirstUid string `json:"first_uid" binding:"required"`
	Name     string `json:"name" binding:"required"`
}


type ProfilesUpdateRequest struct {
	Name string `json:"name,omitempty"`
}


type ProfilesListResponse struct {
	Profiles []ProfilesResponse `json:"profiles"`
	Count    int                `json:"count"`
}
