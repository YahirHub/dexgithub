# Fecha
2026-09-15

# Objetivo
Completar la librería Go standalone `github.com/YahirHub/dexgithub` para conexión GitHub App, descubrimiento de repositorios y operaciones Git seguras reutilizables por Dex.

# Decisiones tomadas
- Go mínimo fijado a `1.25.0` para ser importable por Dex sin elevar su requisito actual.
- Cero dependencias Go externas: GitHub REST/OAuth/JWT/webhook con standard library; operaciones de repositorio mediante el ejecutable `git` del host.
- GitHub App Manifest como flujo de creación guiada de la app.
- Default GitHub API version: `2026-03-10`.
- JWT RS256 implementado con standard library y `iss=Client ID` (App ID como fallback), `iat=-60s` y expiración de 9 minutos.
- OAuth user access token para validar identidad y descubrir las instalaciones realmente accesibles por el usuario.
- `installation_id` nunca se confía por query string: `ValidateUserInstallation` lo contrasta con `/user/installations`.
- Repositorios se agregan desde todas las instalaciones personales/organizaciones accesibles mediante `AccessibleRepositories`.
- Installation tokens expiran en GitHub; DexGitHub los cachea únicamente en memoria y renueva antes del vencimiento.
- Clonado/fetch/pull privado usa `GIT_ASKPASS` temporal con token en entorno del proceso; nunca se incrusta el token en clone URL ni `.git/config`.
- Root de clones por defecto `/root/.github/repos/<owner>/<repo>` y configurable persistentemente.
- Estado privado de la librería bajo `/root/.github/.dexgithub/` con directorios `0700`, archivos `0600`, escritura atómica y rechazo de symlinks en el directorio secreto.
- `Store` se mantiene como interfaz porque representa un límite real de secretos: Dex puede sustituir FileStore por Vault/KMS/otro secret store en el futuro.
- Registro de repos locales no mueve archivos; `ForgetRepository` sólo elimina el registro. `RemoveClonedRepository` sólo borra clones dentro de CloneRoot.
- Git no se ejecuta mediante shell; se usa `exec.CommandContext`, timeout y salida acotada a 8 MiB.
- Pull se limita a `--ff-only` para no crear merges implícitos desde un panel.
- Webhooks incluyen verificación `X-Hub-Signature-256` HMAC-SHA256, pero la librería no crea servidor HTTP ni decide rutas.
- Permiso manifest por defecto: `contents: read`; permisos adicionales son opt-in para futuras funciones.

# Arquitectura actual
Archivos principales:
- `config.go`: defaults, URLs GitHub/GHES, timeouts y Store.
- `models.go`: modelos públicos de app, usuario, instalación, repo, ramas/commits/status.
- `storage.go`: Store + FileStore privado/atómico.
- `client.go`: cliente REST/JSON acotado y errores de API saneados.
- `manifest.go`: GitHub App Manifest y conversión de code.
- `jwt.go`: JWT RS256 de aplicación.
- `auth.go`: OAuth user token, refresh y usuario conectado.
- `installations.go`: instalaciones, repositorios y installation tokens.
- `git.go`: clone/fetch/pull/status/branches/checkout/commits.
- `registry.go`: repositorios conocidos, clones administrados y repos locales.
- `webhook.go`: validación de firma webhook.
- `README.md`: contrato y ejemplos de integración.

# Librerías usadas
Sólo Go standard library. Runtime Git usa el ejecutable `git` del host.

# Archivos importantes modificados
Todo el módulo nuevo `DexGitHub`, incluyendo pruebas, README, contexto y tarea.

# Problemas encontrados
- El parser inicial de ramas eliminaba los campos vacíos de la última referencia por usar `TrimSpace` sobre toda la salida; corregido preservando las líneas crudas.
- La primera validación de componentes de ruta usó una raw string con escapes y trató `r/n/x` como caracteres prohibidos; corregido con string interpretado.
- `/app/installations` devuelve un array mientras `/user/installations` devuelve objeto con `installations`; se separaron ambos parsers.
- `go mod init` tomó Go 1.27.1 del host; se corrigió a Go 1.25.0 para compatibilidad con Dex.

# Soluciones implementadas
- Suite HTTP con `httptest` para manifest, OAuth, identidad, instalaciones personales/organización, repositorios y caché de installation token.
- Tests Git reales en repos temporales para status, ramas, checkout, commits, registro local y clonación sin persistir credenciales.
- Tests de permisos 0700/0600, rechazo de symlink y webhook HMAC.
- API de alto nivel `Service` más `Service.Git` para uso directo cuando sea necesario.

# Pruebas
Pasaron:
- `gofmt -w *.go`
- `go mod tidy`
- `go test ./...`
- `go test -race ./...`
- `go vet ./...`
- `git diff --check`

# Pendientes
- Publicar el repositorio standalone en GitHub cuando exista una sesión/remoto autorizado.
- Integrar la librería en Dex con UI/rutas propias en una tarea futura; no se añadió todavía dependencia a Dex.
- Ejecutar un smoke real contra una GitHub App creada desde Dex cuando la UI de conexión se implemente.

# Próximos pasos
Analizar Dokploy para diseñar en Dex la capa Proyectos → servicios Compose/contenedores, utilizando esta librería como proveedor GitHub y evitando copiar la arquitectura completa de Dokploy.
