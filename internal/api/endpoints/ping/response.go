package ping

type response struct {
	Status       string `json:"status"`
	Version      string `json:"version"`
	BuildID      string `json:"build_id"`
	Digest       string `json:"digest"`
	AttemptToken string `json:"attempt_token"`
}
