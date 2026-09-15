# DexGitHub

Librería Go para conectar un panel o servicio con GitHub mediante **GitHub App**, descubrir repositorios autorizados y administrar clones/repositorios Git locales sin persistir tokens dentro de los remotes.

Pensada inicialmente para Dex, pero el paquete no depende de Dex ni de un framework HTTP.

## Qué resuelve

- Registro guiado de una GitHub App mediante **GitHub App Manifest**.
- Intercambio del `code` del manifest por App ID, Client ID/Secret, private key y webhook secret.
- JWT RS256 de GitHub App con `iat` tolerante a clock drift y expiración corta.
- OAuth de usuario de GitHub App, incluyendo refresh tokens cuando GitHub los entrega.
- Listado de instalaciones accesibles por el usuario, incluyendo cuentas personales y organizaciones.
- Listado agregado de repositorios autorizados por todas las instalaciones del usuario.
- Tokens de instalación de corta duración, cacheados sólo en memoria.
- Clonado de repos públicos y privados.
- Fetch/pull de clones privados obteniendo un token nuevo cuando hace falta.
- Registro de repositorios Git locales ya existentes.
- Configuración del directorio de clones; default: `/root/.github/repos`.
- Estado del working tree, ramas locales/remotas, cambio de rama y lectura de commits.
- Registro persistente de repositorios conocidos.
- Verificación HMAC SHA-256 de webhooks GitHub.
- GitHub Enterprise configurable mediante `WebBaseURL`/`APIBaseURL`.

## Seguridad

DexGitHub evita PATs como mecanismo principal y no incrusta installation tokens en URLs Git. Para Git privado usa `GIT_ASKPASS` temporal: el token existe únicamente en el entorno del proceso Git durante la operación y el `origin` queda como URL limpia.

Las credenciales persistentes de GitHub App se guardan por defecto en:

```text
/root/.github/.dexgithub/app.json
/root/.github/.dexgithub/user.json
/root/.github/.dexgithub/settings.json
/root/.github/.dexgithub/registry.json
```

El directorio queda `0700` y los archivos `0600`. La private key de una GitHub App es un secreto de alto impacto porque permite generar tokens para sus instalaciones. Si el producto final dispone de Vault/KMS/secret manager, puede sustituir el `Store` por una implementación propia.

Los installation tokens no se escriben a disco.

## Requisitos

- Go 1.25+ para consumir la librería.
- `git` disponible en el host para operaciones de repositorios.
- Acceso HTTPS a GitHub/GitHub Enterprise.

No usa CGO ni una librería Git en Go.

## Instalación

Cuando el módulo esté publicado:

```bash
go get github.com/YahirHub/dexgithub
```

## Inicialización

```go
svc, err := dexgithub.New(dexgithub.Config{})
if err != nil {
    return err
}
```

Con configuración por defecto:

```text
RootDir:    /root/.github
CloneRoot:  /root/.github/repos
GitHub web: https://github.com
GitHub API: https://api.github.com
API:        2026-03-10
```

## Flujo recomendado para un panel web

### 1. Crear la GitHub App con Manifest

El backend genera un `state` y lo guarda en la sesión web del administrador.

```go
state, _ := dexgithub.RandomState()
form, err := svc.ManifestForm(dexgithub.ManifestOptions{
    Name:         "Dex GitHub",
    HomepageURL:  "https://dex.example.com",
    RedirectURL:  "https://dex.example.com/github/manifest/callback",
    CallbackURLs: []string{"https://dex.example.com/github/oauth/callback"},
    RequestOAuth: true,
    State:        state,
})
```

La UI debe enviar un `POST` a `form.ActionURL` con:

```text
manifest=<form.Manifest>
```

GitHub devuelve `code` + `state` a `RedirectURL`. El panel verifica el `state` de su sesión y después:

```go
credentials, err := svc.CompleteManifest(ctx, code)
```

### 2. Instalar la GitHub App

```go
installURL, err := svc.InstallURL()
```

Redirigir al usuario a esa URL. GitHub permite seleccionar todas las repositories o sólo algunas.

### 3. Autorizar al usuario

Si se usó `RequestOAuth: true`, GitHub solicitará autorización durante el flujo de instalación. También puede iniciarse manualmente:

```go
state, _ := dexgithub.RandomState()
url, err := svc.UserAuthorizationURL(dexgithub.AuthorizeOptions{
    RedirectURI: "https://dex.example.com/github/oauth/callback",
    State:       state,
})
```

En el callback, después de verificar `state`:

```go
token, err := svc.ExchangeUserCode(ctx, code, "https://dex.example.com/github/oauth/callback")
```

El token se valida inmediatamente contra `GET /user` antes de persistirlo.

### 4. Ver cuentas/organizaciones y repositorios

```go
installations, err := svc.UserInstallations(ctx)
repos, err := svc.AccessibleRepositories(ctx)
```

Cada `Repository` incluye `InstallationID`, necesario para clonar repos privados sin PAT.

Si GitHub redirige una instalación con `installation_id`, no confiar directamente en ese query param. Verificarlo:

```go
installation, err := svc.ValidateUserInstallation(ctx, installationID)
```

### 5. Clonar repo privado

```go
record, err := svc.CloneRepository(ctx, installationID, repositoryID, "")
```

Con destino vacío se usa:

```text
/root/.github/repos/<owner>/<repo>
```

El token de instalación se solicita en ese momento, vive sólo en memoria y no queda en `.git/config`.

### 6. Clonar repo público sin conexión GitHub App

```go
record, err := svc.ClonePublic(ctx, "https://github.com/OWNER/REPO.git", "")
```

### 7. Registrar un repo local

```go
record, err := svc.RegisterLocal(ctx, "/srv/proyectos/api")
```

Registrar no mueve ni copia el repositorio.

## Operaciones Git

Repositorios registrados:

```go
repos, err := svc.KnownRepositories()
```

Estado:

```go
status, err := svc.RepositoryStatus(ctx, record.ID)
```

Fetch privado/público:

```go
err := svc.FetchRepository(ctx, record.ID)
```

Pull seguro sólo fast-forward:

```go
err := svc.PullRepository(ctx, record.ID)
```

Ramas:

```go
branches, err := svc.RepositoryBranches(ctx, record.ID)
```

Cambiar rama:

```go
err := svc.CheckoutRepositoryBranch(ctx, record.ID, "main")
```

Si la rama existe sólo como `origin/<rama>`, se crea la rama local con tracking.

Commits:

```go
commits, err := svc.RepositoryCommits(ctx, record.ID, "HEAD", 50)
```

Máximo por consulta: 500.

Olvidar un repo local sin borrar sus archivos:

```go
err := svc.ForgetRepository(record.ID)
```

Eliminar un clone administrado:

```go
err := svc.RemoveClonedRepository(ctx, record.ID)
```

Esta última operación sólo elimina registros de tipo `clone` cuya ruta siga dentro del `CloneRoot` administrado.

## Cambiar el directorio de clones

```go
err := svc.SetCloneRoot(ctx, "/srv/dex/github")
```

El cambio afecta clones futuros. No mueve automáticamente clones ya registrados.

## Webhooks

Si el Manifest habilita webhook, validar siempre `X-Hub-Signature-256` antes de procesar el body:

```go
ok, err := svc.VerifyWebhook(body, r.Header.Get("X-Hub-Signature-256"))
```

La librería no crea un servidor HTTP de webhooks: sólo proporciona la primitiva de verificación para que el host conserve control de rutas, CSRF, auditoría y autorización.

## Permisos GitHub App

El default de `ManifestForm` solicita únicamente:

```text
Contents: read
```

Es suficiente para descubrir y clonar el contenido autorizado. Permisos adicionales deben pedirse sólo cuando una función futura los necesite.

El manifest acepta permisos/eventos personalizados mediante `ManifestOptions.Permissions` y `ManifestOptions.Events`.

## API pública principal

```text
New
RandomState
Service.ManifestForm
Service.CompleteManifest
Service.InstallURL
Service.UserAuthorizationURL
Service.ExchangeUserCode
Service.ConnectedUser
Service.LogoutGitHub
Service.UserInstallations
Service.AppInstallations
Service.ValidateUserInstallation
Service.RepositoriesForUserInstallation
Service.RepositoriesForInstallation
Service.AccessibleRepositories
Service.CloneRepository
Service.ClonePublic
Service.RegisterLocal
Service.KnownRepositories
Service.RepositoryStatus
Service.FetchRepository
Service.PullRepository
Service.RepositoryBranches
Service.CheckoutRepositoryBranch
Service.RepositoryCommits
Service.ForgetRepository
Service.RemoveClonedRepository
Service.SetCloneRoot
Service.VerifyWebhook
VerifyWebhookSignature
```

También se expone `Service.Git` para operaciones de bajo nivel sobre rutas Git cuando el consumidor no necesita el registro persistente.

## Diseño deliberado

- No usa GitHub CLI para el runtime.
- No usa PATs persistentes.
- No persiste installation tokens.
- No ejecuta `sh -c` para comandos Git.
- No intenta hacer push: el alcance inicial pedido es lectura/clonado/despliegue.
- No implementa una base de datos; usa JSON privado y atómico para un único host.
- No implementa cola/background jobs: el caller controla `context.Context` y timeouts.

## Verificación

```bash
gofmt -w *.go
go test ./...
go test -race ./...
go vet ./...
git diff --check
```

## Documentación oficial de referencia

- GitHub App Manifest: https://docs.github.com/en/apps/sharing-github-apps/registering-a-github-app-from-a-manifest
- Autenticación GitHub App: https://docs.github.com/en/apps/creating-github-apps/authenticating-with-a-github-app
- JWT GitHub App: https://docs.github.com/en/apps/creating-github-apps/authenticating-with-a-github-app/generating-a-json-web-token-jwt-for-a-github-app
- User access tokens: https://docs.github.com/en/apps/creating-github-apps/authenticating-with-a-github-app/generating-a-user-access-token-for-a-github-app
- Installation tokens: https://docs.github.com/en/apps/creating-github-apps/authenticating-with-a-github-app/generating-an-installation-access-token-for-a-github-app
