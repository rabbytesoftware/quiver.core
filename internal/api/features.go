package api

// features lists the optional capabilities this daemon serves, advertised on
// GET /versions so a client can tell what an older daemon lacks.
var features = []string{"console.v1"}
