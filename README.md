# mira-commerce-plugin-example

**Template oficial de plugin do [Mira Commerce](https://mira-dev.tech).**
Um programa de fidelidade mínimo, comentado linha a linha, que você clona,
roda em 30 segundos e transforma no seu próprio plugin.

O que ele demonstra — as três superfícies que quase todo plugin real usa:

| Superfície | Hook/Evento | O que o exemplo faz |
|------------|-------------|---------------------|
| Hook que **ajusta** um resultado | `checkout.quote_adjust` | 5% de desconto quando o subtotal ≥ R$ 300 |
| Hook que **bloqueia** um fluxo | `order.pre_confirm` | Nega pedido acima de um teto (código estável `LOYALTY_ORDER_CAP`) |
| **Evento assíncrono** | `order.confirmed` | Credita pontos — idempotente pela chave de negócio (`order_id`) |

Exemplo concreto: um pedido de **R$ 879,90** sai do checkout por
**R$ 835,905** (desconto de R$ 43,995 aplicado pelo plugin, antes do pedido
ser persistido).

> A documentação completa do protocolo (catálogo de 18 hooks, 14 eventos,
> manifest, semânticas de falha) está no README do SDK:
> [`commerce-ext`](https://github.com/mira-dev-tech/commerce-ext).

---

## Comece em 30 segundos

```bash
git clone https://github.com/mira-dev-tech/mira-commerce-plugin-example
cd mira-commerce-plugin-example
go test ./...        # funciona direto — o SDK está vendorado, zero setup
```

## Tour pelos arquivos (nesta ordem)

| Arquivo | O que você aprende lendo |
|---------|--------------------------|
| [`plugin.go`](plugin.go) | O plugin inteiro: `Meta/Init/Register/Shutdown`, os 2 hooks, o handler de evento, config via `Runtime` → env → default. **Cada decisão está comentada no lugar onde acontece.** |
| [`plugin_test.go`](plugin_test.go) | Testes unitários usando só a superfície do SDK — inclusive o teste que trava a paridade entre `Register` e o `manifest.yaml` |
| [`manifest.yaml`](manifest.yaml) | Como declarar capacidades (hooks/eventos) e compatibilidade de versão |
| [`cmd/example-loyalty/main.go`](cmd/example-loyalty/main.go) | Como empacotar o plugin como **binário externo** (go-plugin/RPC) — são 3 linhas |
| [`.github/workflows/ci.yml`](.github/workflows/ci.yml) | CI mínimo: vet + test + build + handshake |

## Os dois modos de rodar um plugin

### Modo 1 — in-process (compilado dentro do core)

Para plugins mantidos junto da plataforma: o pacote é copiado para o monorepo
do core (`extensions/<id>`) e registrado no loader. O manifest usa
`runtime.type: in-process`. *(Requer acesso ao repositório interno do core.)*

### Modo 2 — binário externo (go-plugin/RPC)

Para código que **não** deve ser compilado no core — ex.: plugin de
cliente/parceiro. O core sobe o seu binário como processo filho e conversa por
RPC; o binário importa **apenas** o SDK público:

```bash
go build -o bin/example-loyalty ./cmd/example-loyalty

# probe do protocolo — deve imprimir: commerce-ext-ok
./bin/example-loyalty --commerce-ext-handshake

# no deploy do core, aponte o manifest deste plugin:
MIRA_MANIFEST_PATH=/caminho/para/manifest.yaml <processo do core>
```

O host valida o handshake, registra os hooks enumerados pelo binário e faz a
ponte de cada chamada por RPC. Se o processo do plugin morrer, os hooks
degradam com código explícito `plugin_failure` — nunca um bloqueio silencioso.

## Crie o seu plugin a partir deste template

1. **Fork/clone** com o nome `mira-commerce-plugin-<seu-id>`.
2. **Renomeie a identidade**: `id` no `Meta()` e no `manifest.yaml`, o nome do
   pacote, o prefixo de env (`EXAMPLE_LOYALTY_*` → `SEU_PLUGIN_*`).
3. **Escolha hooks/eventos** no catálogo do SDK e **declare no manifest** — o
   teste `TestRegisterDeclaresManifestCapabilities` falha se divergirem.
4. **Escreva o handler mais burro possível primeiro**, com teste, e só depois
   sofistique. O exemplo mostra o padrão para cada tipo de handler.
5. **Rode `go test ./...` + o handshake** antes de qualquer deploy.

### As 5 regras de ouro (aprendidas em produção)

1. **Hook é síncrono com timeout de 300ms** — nada de I/O bloqueante; se
   estourar, o host degrada com `plugin_failure` (fail-closed em hooks de
   bloqueio).
2. **Handler de evento é idempotente pela CHAVE DE NEGÓCIO** (`order_id`),
   nunca pelo `Event.ID` — a entrega é at-least-once e o mesmo fato pode
   chegar em mais de um evento com IDs diferentes. Veja `onOrderConfirmed` no
   `plugin.go`.
3. **Código de bloqueio é contrato** — `LOYALTY_ORDER_CAP` é lido por front e
   suporte; publicou, não renomeia.
4. **Estado persistente vai para banco próprio do plugin** (schema
   `ext_<plugin>` no Postgres da plataforma). O contador em memória deste
   exemplo existe só para fins didáticos.
5. **Config via `Runtime.Config` → env → default; secrets nunca em log nem
   hardcode.**

## Atualizando a versão do SDK

O diretório `vendor/` congela o `commerce-ext` para o build funcionar sem
setup. Para subir de versão:

```bash
go get github.com/mira-dev-tech/commerce-ext@vX.Y.Z
go mod tidy && go mod vendor
go test ./...
```

## Troubleshooting

| Sintoma | Causa provável |
|---------|----------------|
| `--commerce-ext-handshake` não imprime `commerce-ext-ok` | O `main` não chama `commerceext.Serve(...)`, ou o binário é de outra arquitetura |
| Host recusa o plugin no load | `compatibleCore` do manifest incompatível com a linha do protocolo do core, ou hook usado sem estar declarado em `capabilities` |
| Hook "não dispara" | Confira o log do core (`ext: go-plugin RPC loaded id=...` / `ext: plugins loaded [...]`) e a prioridade — outro plugin pode ter bloqueado a cadeia antes |
| Checkout bloqueado com `plugin_failure` | Seu handler estourou 300ms, deu panic, ou o processo externo morreu — veja o log do core (`ext/rpc: hook ... failed`) |
| Evento processado 2×, pontos duplicados | Dedupe por `Event.ID` em vez da chave de negócio — releia a regra de ouro nº 2 |

## Sobre este repositório

Código de **exemplo didático**, fornecido como está. Forks são bem-vindos para
criar seus próprios plugins; a escrita aqui é restrita à equipe Mirá para o
template permanecer um ponto de partida estável.
