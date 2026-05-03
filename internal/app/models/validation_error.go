package models

type ValidationError struct {
	Field   string
	Rule    string
	Message string
}
