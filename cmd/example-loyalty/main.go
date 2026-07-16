// Command example-loyalty empacota o plugin como BINÁRIO EXTERNO (go-plugin):
// o Extension Host do core sobe este processo e conversa com ele por RPC,
// sem compilar o código do plugin dentro do core.
//
// Build:  go build -o bin/example-loyalty ./cmd/example-loyalty
// Teste:  ./bin/example-loyalty --commerce-ext-handshake   → imprime "commerce-ext-ok"
// Carga:  manifest.yaml (runtime.type go-plugin) + MIRA_MANIFEST_PATH no core.
package main

import (
	commerceext "github.com/mira-dev-tech/commerce-ext"

	exampleloyalty "github.com/mira-dev-tech/mira-commerce-plugin-example"
)

func main() {
	commerceext.Serve(exampleloyalty.New())
}
