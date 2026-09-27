package hosts

import "errors"

var ErrRawNotFound = errors.New("hosts: raw file not found")

var ErrUnexpectedPage = errors.New("hosts: page did not match the expected shape")

var ErrReleaseNotFound = errors.New("hosts: release not found")
