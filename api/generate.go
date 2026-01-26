package api

//go:generate oapi-codegen -package api -generate types -o types.gen.go openapi.yaml
//go:generate oapi-codegen -package api -generate chi-server -o server.gen.go openapi.yaml
