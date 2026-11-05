# APISIX Dashboard

Dashboard para o [Apache APISIX](https://apisix.apache.org/) (modo tradicional com etcd), pensada para ambientes PCI DSS.
É um binário único, com o frontend embutido, que fala só com a **Admin API**. A admin key fica no servidor e nunca vai para o navegador.

```
navegador ──HTTPS──▶ apisix-dashboard ──X-API-KEY──▶ APISIX Admin API :9180 ──▶ etcd
                       │ sessão, MFA, RBAC, auditoria
                       └ SQLite (usuários, sessões, auditoria)
```

## Início rápido

```bash
make build                      # requer Node 24+ e Go 1.26+
export DASH_ADMIN_API_URL=http://127.0.0.1:9180
export DASH_ADMIN_API_KEY=sua-admin-key
./bin/apisix-dashboard user add -u admin -r admin    # pede a senha
./bin/apisix-dashboard                               # http://localhost:8080
```

No primeiro login, o usuário configura o MFA (TOTP) com qualquer app autenticador.

> Em HTTP puro, fora de `localhost`, o navegador não envia o cookie de sessão (`Secure`). Para testes locais sem TLS use `DASH_COOKIE_SECURE=false`.

## Configuração

Toda a configuração vem de variáveis de ambiente. As variáveis com segredo aceitam o sufixo `_FILE` (compatível com Docker secrets).

| Variável | Padrão | Descrição |
|---|---|---|
| `DASH_ADMIN_API_URL` | — | URL da Admin API, ex.: `http://apisix:9180` (**obrigatória**) |
| `DASH_ADMIN_API_KEY` / `_FILE` | — | Admin key do APISIX. Opcional apenas para APISIX de teste com `admin_key_required: false`; em produção, sempre defina |
| `DASH_ADMIN_API_CA_FILE` | — | CA para Admin API em HTTPS |
| `DASH_ADMIN_API_TIMEOUT` | `10s` | Timeout das chamadas à Admin API |
| `DASH_LISTEN_ADDR` | `:8080` | Endereço de escuta |
| `DASH_TLS_CERT_FILE` / `DASH_TLS_KEY_FILE` | — | Ativam HTTPS (TLS 1.2+) |
| `DASH_DATA_DIR` | `./data` | Diretório do SQLite e da chave gerada |
| `DASH_ENCRYPTION_KEY_FILE` | `$DATA_DIR/encryption.key` | Chave AES-256 (32 bytes, hex/base64) que cifra os segredos TOTP. Se não for informada, uma chave é gerada no diretório de dados |
| `DASH_SESSION_IDLE_TIMEOUT` | `15m` | Timeout por inatividade (PCI 8.2.8) |
| `DASH_SESSION_MAX_LIFETIME` | `12h` | Duração máxima da sessão |
| `DASH_MAX_LOGIN_ATTEMPTS` | `6` | Falhas (senha ou MFA) até o bloqueio (PCI 8.3.4: no máximo 10) |
| `DASH_LOCKOUT_DURATION` | `30m` | Duração do bloqueio (PCI 8.3.4: no mínimo 30 minutos) |
| `DASH_COOKIE_SECURE` | `true` | Cookie só via HTTPS |
| `DASH_TRUST_PROXY_HEADERS` | `false` | Usa `X-Forwarded-For` para o IP da auditoria (só atrás de proxy confiável) |
| `DASH_AUDIT_LOG` | `stdout` | Cópia da auditoria em JSON lines: `stdout`, `none` ou caminho de arquivo |
| `DASH_ACME_CLOUDFLARE_API_TOKEN` | — | Ativa os certificados automáticos (aceita `_FILE`) |
| `DASH_ACME_EMAIL` | — | Contato da conta no Let's Encrypt (opcional) |
| `DASH_ACME_RENEW_BEFORE` | `720h` | Antecedência da renovação (limitada a 1/3 da validade do certificado) |
| `DASH_ACME_DIRECTORY` / `DASH_ACME_STAGING_DIRECTORY` | Let's Encrypt | Diretórios ACME de produção e staging |
| `DASH_ACME_CA_FILE` | — | CA extra para falar com o diretório ACME (CA interna) |
| `DASH_ACME_DNS_RESOLVER` | NS da zona | Servidor `host:porta` usado para conferir a propagação do TXT |
| `DASH_ACME_PROPAGATION_TIMEOUT` | `2m` | Espera máxima pela propagação antes de pedir a validação |
| `DASH_KONG_ADMIN_URL` | — | Admin API do Kong, ex.: `http://kong-admin:8001`. Ativa a migração do Kong |
| `DASH_KONG_ADMIN_TOKEN` / `_FILE` | — | Token da Admin API do Kong, se exigido |
| `DASH_KONG_ADMIN_TOKEN_HEADER` | `Kong-Admin-Token` | Header em que o token é enviado |
| `DASH_KONG_WORKSPACE` | — | Workspace (Kong Enterprise) |
| `DASH_KONG_CA_FILE` | — | CA para a Admin API do Kong em HTTPS |
| `DASH_KONG_TIMEOUT` | `30s` | Timeout das chamadas ao Kong |

## Perfis

| Perfil | Pode |
|---|---|
| `viewer` | Visualizar a configuração do APISIX |
| `operator` | `viewer` + criar, alterar e excluir recursos do APISIX |
| `admin` | `operator` + gerenciar usuários da dashboard e consultar a auditoria |

## Controles de segurança e PCI DSS 4.0

| Requisito | Implementação |
|---|---|
| 3 / 8.3.2: proteção de dados sensíveis | Segredos são **mascarados** (`******`) em toda resposta e na auditoria. A dashboard usa os `encrypt_fields` que cada plugin declara ao APISIX (cobertura automática de 44 plugins, incluindo campos aninhados) e também uma lista fixa (senhas, chaves privadas de SSL e mTLS, token do Vault...). Ao salvar com o valor mascarado, o backend recupera o valor original do APISIX. Referências `$secret://` e `$env://` aparecem normalmente |
| 7.2: menor privilégio | Perfis `viewer`, `operator` e `admin`, aplicados no backend. Só os recursos da lista de permitidos ficam acessíveis na Admin API |
| 8.2.8: timeout de sessão | Timeout de 15 min por inatividade e duração máxima de 12 h. Sessões ficam no servidor, com o token guardado como hash |
| 8.3.1 / 8.4: MFA | TOTP (RFC 6238) obrigatório para todos os usuários, com proteção contra replay. O segredo é cifrado com AES-256-GCM |
| 8.3.4: bloqueio de conta | Bloqueio por 30 min após 6 falhas de senha ou de MFA |
| 8.3.5: primeira senha | Senha definida por admin é temporária, e o usuário precisa trocá-la no primeiro acesso |
| 8.3.6 / 8.3.7: senhas | Mínimo de 12 caracteres, com letras e números. Argon2id. Não pode repetir as 4 últimas |
| 10.2: trilha de auditoria | Login, logout, falhas, bloqueios, MFA, gestão de usuários (incluindo ações via CLI) e toda alteração no APISIX, com estado **antes e depois**, usuário, IP e resultado |
| 10.3.3: proteção dos logs | Os eventos são espelhados em JSON no stdout/arquivo para envio ao SIEM |
| 6.4: aplicações web | CSP restritiva, proteção contra CSRF (token + `Sec-Fetch-Site`/Origin), cookies `HttpOnly`/`Secure`/`SameSite=Strict`, HSTS, `X-Frame-Options` |

**O que fica por conta da operação:** envio dos logs ao SIEM, backup do volume `/data` e da chave de criptografia, TLS (direto na dashboard ou num proxy), isolamento da Admin API numa rede interna e revisão periódica dos acessos (8.2.6: desativar usuários inativos há mais de 90 dias).

## Docker / Swarm

```bash
make docker                       # imagem distroless, usuário não-root
```

Veja `deploy/docker-stack.yml`, que usa secrets para a admin key, a chave de criptografia e o TLS. Como o estado fica em SQLite, rode **uma réplica**, fixada no nó do volume.

## CLI

```
apisix-dashboard user add -u NOME -r PERFIL   # senha via prompt ou stdin
apisix-dashboard user passwd -u NOME          # senha temporária
apisix-dashboard user reset-mfa -u NOME
apisix-dashboard user unlock -u NOME
apisix-dashboard user list
apisix-dashboard healthcheck
```

## Desenvolvimento

```bash
make test vet
make audit          # govulncheck + npm audit (PCI 6.3)
cd web && npm run dev   # Vite em :5173 com proxy de /api para :8080
```

Estrutura:

```
cmd/apisix-dashboard/   main + CLI
internal/config         variáveis DASH_*
internal/auth           senhas, TOTP, sessões, bloqueio, perfis
internal/apisix         cliente da Admin API, lista de permitidos, mascaramento e catálogo de schemas
internal/kong           leitura do Kong, tradução para o APISIX e aplicação por etapas
internal/audit          trilha de auditoria
internal/store          SQLite
internal/server         HTTP API + middlewares de segurança
internal/web            frontend embutido (gerado por web/)
web/                    React + TypeScript + Mantine
  src/schema/           gerador de formulários a partir de JSON Schema + editor de plugins
  src/forms/            formulários por recurso
```

## Formulários

Cada recurso tem um formulário próprio para os campos do dia a dia e uma aba **JSON** que edita o mesmo objeto. Campos que o APISIX conhece mas o formulário não trata explicitamente aparecem em "Avançado", gerados a partir do schema do próprio APISIX (`/apisix/admin/schema/*`).

Os plugins usam um formulário gerado a partir do schema de cada um (`/apisix/admin/plugins?all=true`), então acompanham a versão do APISIX em uso, inclusive plugins novos. Campos sensíveis viram campo de senha. Os consumers usam o `consumer_schema` dos plugins de autenticação.

**Credenciais de consumers** (APISIX 3.10+) ficam no painel do consumer (menu da linha → *Credenciais*). Cada consumer pode ter várias credenciais (key-auth, basic-auth, jwt-auth, hmac-auth…), por exemplo uma por aplicação, para trocar ou revogar uma sem afetar as outras. O formulário só oferece plugins de autenticação. Os segredos aparecem mascarados e, se mantidos como `******` na edição, preservam o valor atual. Cada alteração entra na auditoria como `consumers` / `usuario/credentials/id`.

## Certificados automáticos (Let's Encrypt)

A dashboard emite e renova certificados pelo desafio **DNS-01 na Cloudflare** e os publica no APISIX como objetos SSL, aplicados sem reload. Não há acme.sh, volume de certificados nem bloco `server` no `config.yaml`. Wildcards são suportados.

Fluxo de cada emissão, visível em tempo real na tela **Certificados automáticos**:

1. Pedido na CA → TXT `_acme-challenge` criado na Cloudflare.
2. Espera a propagação nos servidores autoritativos da zona → a CA valida.
3. Chave nova gerada (ECDSA P-256 ou RSA 2048) → certificado conferido (cobre os domínios, bate com a chave) → `PUT /ssls/{id}`.
4. TXT removido, inclusive quando a emissão falha.

Renovação automática 30 dias antes do vencimento (verificada de hora em hora). Falhas tentam de novo com espera crescente (1h, 2h, 4h… até 24h), respeitando os limites do Let's Encrypt. Use o ambiente **staging** para testar antes de emitir em produção.

Segurança:

- **Chave privada:** gerada a cada emissão e enviada só ao APISIX. Nunca é gravada na dashboard nem aparece na auditoria. No etcd, ela fica cifrada pelo `apisix.data_encryption.keyring`. **Troque o keyring padrão do APISIX**, porque ele é público.
- **Conta ACME:** a chave fica cifrada com a chave de criptografia da dashboard.
- **Token da Cloudflare:** use um token com permissão **Zone · DNS · Edit** só nas zonas necessárias, entregue como Docker secret (`DASH_ACME_CLOUDFLARE_API_TOKEN_FILE`).
- **Proteção contra edição manual:** um SSL emitido automaticamente fica bloqueado para edição e exclusão pelas telas comuns, para que a próxima renovação não desfaça uma alteração manual sem ninguém perceber. Ao remover o certificado automático, você escolhe entre manter o SSL (que vira um SSL comum) ou excluí-lo.
- **Auditoria:** cadastro, cada emissão (manual ou `acme:renovação-automática`, com fingerprint antes e depois) e falhas.

Saída de rede necessária (para regras de firewall no PCI):

- HTTPS para `acme-v02.api.letsencrypt.org` e `api.cloudflare.com`.
- DNS (porta 53) para os name servers da zona, usado na conferência da propagação. Se essa porta estiver bloqueada, a dashboard só registra um aviso e segue com a validação.
- Proxy de saída: `HTTPS_PROXY` é respeitado.

Para **migrar um certificado existente**, cadastre os domínios com o mesmo ID do SSL e marque "substituir". O certificado atual continua sendo servido até o novo ser emitido.

## Migração do Kong

A tela **Migração → Kong → APISIX** (perfis `operator` e `admin`) traz a configuração de um Kong 3.x com banco de dados para o APISIX em etapas. A dashboard só **lê** do Kong.

1. **Padrões:** definem como tudo é traduzido e valem para todas as etapas: estratégia de ID (nome ou ID do Kong) e prefixo, rotas criadas desabilitadas (para ativar no cutover), o que fazer com objetos que já existem no APISIX, plugins sem tradução (bloquear ou migrar sem), reescrita de hosts (ex.: DNS do Kong → nome do serviço no Swarm), labels extras e **regras de plugins**.
2. **Inventário:** services com upstream, rotas e plugins; consumers com credenciais; plugins globais. Cada item mostra se está a migrar, migrado, alterado no Kong desde a migração ou bloqueado, com os avisos da tradução.
3. **Pré-visualização:** o JSON exato de cada objeto e, se já existir no APISIX, o atual lado a lado. Os segredos aparecem mascarados.
4. **Aplicar:** grava na ordem upstreams → services → consumers → credenciais → rotas → global rules. Se um objeto falhar, o que o lote gravou é desfeito. O apply só é aceito se o Kong e o APISIX ainda estiverem como na pré-visualização.

Selecionar um service leva o upstream, os plugins e todas as rotas. Também dá para escolher rotas avulsas. Rodar de novo é seguro: o que não mudou aparece como "migrado" e não é regravado.

| Kong | APISIX |
|---|---|
| Service + Upstream/Targets | `upstream` (nós, balanceamento, health checks, timeouts, retries, `pass_host`) + `service` |
| Route | `route`: paths por prefixo viram `uris` com `*`, paths regex (`~`) e headers viram `vars`, e `strip_path` + path do service viram `proxy-rewrite.regex_uri` |
| Consumer + key-auth, jwt, hmac-auth | `consumer` + credenciais. Os grupos de ACL viram o label `groups` |
| Plugins globais | `global_rules` |

Plugins traduzidos: `key-auth`, `basic-auth`, `jwt`, `hmac-auth`, `acl` (plugin `acl` do APISIX), `rate-limiting` (`limit-count`, com uma regra por janela), `cors`, `ip-restriction`, `request-transformer` e `response-transformer` (só headers), `correlation-id`, `prometheus`, `file-log`, `http-log`, `tcp-log`, `udp-log`, `proxy-cache`, `request-size-limiting`, `bot-detection`, `request-termination` e `openid-connect`. Diferenças de comportamento viram avisos no inventário.

**Plugins custom:** crie uma regra em Padrões que mapeia o plugin do Kong para o do APISIX com um template JSON. Por exemplo, `{"nivel": "${config.nivel}", "limite": "${config.limite|10}", "rota": "${apisix.route_id}"}`. A regra vale para todas as ocorrências do plugin, e também pode simplesmente ignorá-lo.

Limitações conhecidas:

- As senhas de **basic-auth** não migram, porque o Kong guarda só o hash.
- O header de assinatura do **hmac-auth** é diferente entre os dois gateways.
- Services TCP/UDP (stream), rotas por expressão e plugins por consumer group não são suportados.
- Se uma credencial for recriada no Kong, a antiga continua no APISIX: a pré-visualização avisa quando a chave colide.

Cada gravação entra na auditoria como `kong.migrate`, com antes e depois. Rollbacks entram como `kong.rollback`.

## Roadmap

- Login via OIDC (Keycloak), mantendo o login local como contingência
- Plugin metadata na interface
- Integração com o Swarm: descobrir serviços e sugerir upstreams
- Histórico e rollback de configurações a partir da auditoria
