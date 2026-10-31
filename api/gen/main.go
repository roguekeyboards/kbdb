// Command gen regenerates api/gateway.gen.yaml, the HTTP API's routes, from
// api/openapi.yaml. Run it through `mise run gen`; check-generated (and CI)
// catch a stale file.
package main

import (
	"flag"
	"log"
	"os"

	"github.com/getkin/kin-openapi/openapi3"

	"github.com/roguekeyboards/mykeebs-api/internal/gatewayroutes"
)

func main() {
	specPath := flag.String("spec", "api/openapi.yaml", "REST API spec to read")
	out := flag.String("out", "api/gateway.gen.yaml", "HTTP API definition to write")
	flag.Parse()
	if err := run(*specPath, *out); err != nil {
		log.Fatal(err)
	}
}

func run(specPath, out string) error {
	spec, err := openapi3.NewLoader().LoadFromFile(specPath)
	if err != nil {
		return err
	}
	routes, err := gatewayroutes.Routes(spec)
	if err != nil {
		return err
	}
	body, err := gatewayroutes.Definition(routes)
	if err != nil {
		return err
	}
	return os.WriteFile(out, body, 0o600)
}
