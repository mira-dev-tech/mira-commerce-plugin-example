# mira-commerce-plugin-example

Template de **plugin do Mira Commerce**: um programa de fidelidade mínimo,
comentado linha a linha, para você copiar e transformar no seu plugin.

O que ele demonstra:

| Superfície | Hook/Evento | O que faz |
|------------|-------------|-----------|
| Hook que **ajusta** resultado | `checkout.quote_adjust` | 5% de desconto quando subtotal ≥ R$ 300 |
| Hook que **bloqueia** fluxo | `order.pre_confirm` | Nega pedido acima de um teto (`LOYALTY_ORDER_CAP`) |
| Evento **assíncrono** | `order.confirmed` | Credita pontos — idempotente pela chave de negócio (`order_id`) |

> Guia completo do modelo (contrato, catálogo de hooks, prioridades, manifest,
> persistência, checklist, anti-patterns):
> [`docs/plugin-example.md`](https://github.com/mira-dev-tech/mira-commerce-core/blob/main/docs/plugin-example.md)
> no mira-commerce-core.

## Estrutura

```
plugin.go                  # o plugin (package exampleloyalty) — comece por aqui
plugin_test.go             # testes unitários (só a superfície commerce-ext)
cmd/example-loyalty/       # empacota como binário externo (go-plugin/RPC)
manifest.yaml              # capacidades declaradas (validado pelo core)
vendor/                    # commerce-ext vendorado — build e CI sem credencial
```

A única dependência é o SDK público
[`commerce-ext`](https://github.com/mira-dev-tech/commerce-ext) — um plugin
**nunca** importa `mira-commerce-core/internal/...`.

## Rodar os testes

```bash
go test ./...
```

Funciona direto após o clone (o SDK está vendorado).

## Dois modos de rodar o plugin

### 1. In-process (compilado dentro do core) — o modo padrão hoje

Copie o pacote para o monorepo e registre no loader:

```bash
cp plugin.go plugin_test.go manifest.yaml \
   mira-commerce-core/extensions/<meu-plugin>/
# registrar em internal/ext/loader.go (Bootstrap) — ver exemplo do
# example-loyalty, que carrega atrás de MIRA_EXAMPLE_PLUGIN=1
```

No manifest, use `runtime.type: in-process`. É assim que os plugins de
referência (`reference-antifraud`, `reference-erp`) e o próprio
`example-loyalty` rodam no core.

### 2. Binário externo (go-plugin/RPC) — código fora do core

```bash
go build -o bin/example-loyalty ./cmd/example-loyalty
./bin/example-loyalty --commerce-ext-handshake   # deve imprimir commerce-ext-ok

# no core, aponte o manifest deste repo:
MIRA_MANIFEST_PATH=/caminho/para/este/repo/manifest.yaml make dev-api
```

O Extension Host valida o handshake, sobe o binário e faz a ponte dos hooks
por RPC. O binário importa **só** o commerce-ext — é o contrato para plugin de
cliente sem fork do core.

## Criando o seu plugin a partir deste template

1. Clone/copie este repo com o nome `mira-commerce-plugin-<id>`.
2. Renomeie `id`, `package`, env prefix e o código de bloqueio (`LOYALTY_ORDER_CAP`
   é contrato do exemplo — o seu terá outro, e ele é estável: não renomeie sem ADR).
3. Escolha hooks/eventos no catálogo (`hooks.go`/`events.go` do commerce-ext) e
   **declare-os no manifest** — o teste `TestRegisterDeclaresManifestCapabilities`
   trava a paridade.
4. Regras que valem sempre:
   - hooks são síncronos, 300ms de timeout, panic-safe — nada de I/O bloqueante;
   - handler de evento é idempotente **pela chave de negócio** (entrega
     at-least-once e o mesmo fato pode virar mais de um evento);
   - estado persistente vai para Postgres em schema `ext_<plugin>` (ADR 0004) —
     o contador em memória aqui é só didático;
   - config via `Runtime.Config` → env → default; nunca hardcode nem log de secret.
5. Atualize o `vendor/` quando subir a versão do SDK:
   ```bash
   export GOPRIVATE=github.com/mira-dev-tech
   go get github.com/mira-dev-tech/commerce-ext@vX.Y.Z && go mod tidy && go mod vendor
   ```

## CI

`go vet` + `go test` + build do binário em cada push (sem segredos — o SDK está
vendorado).
