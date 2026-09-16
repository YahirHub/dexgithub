# Fecha
2026-09-15

# Objetivo
Permitir que DexGitHub genere y procese callbacks Manifest/OAuth sobre HTTP cuando el destino sea un host local o de LAN confiable, incluyendo `http://dex.local:9090`, sin relajar la política para hosts públicos ni webhooks.

# Decisiones tomadas
- HTTPS sigue siendo aceptado para cualquier host.
- HTTP sólo se permite cuando el caller habilita URL local y el host es loopback, IP privada/link-local, `.local` o `.home.arpa`.
- Un hostname público por HTTP continúa rechazado.
- Los webhooks mantienen HTTPS obligatorio.
- `WebBaseURL` y `APIBaseURL` de GitHub/GHES conservan su validación existente; este cambio aplica a URLs de retorno de Manifest/OAuth.

# Arquitectura actual
La validación de URL permanece en `requireHTTPSURL`; se añadió una clasificación mínima `isTrustedLocalHTTPHost` basada en stdlib (`net.ParseIP`, `IP.IsPrivate`, `IP.IsLinkLocalUnicast`) y sufijos locales explícitos.

# Librerías usadas
Sólo Go standard library; se añadió uso de `net`.

# Archivos importantes modificados
- `manifest.go`
- `github_test.go`
- `README.md`
- `tareas/completado-02-permitir-callbacks-http-lan.md`

# Problemas encontrados
La versión `v0.1.0` aceptaba HTTP únicamente para loopback, por lo que un Dex configurado como `http://dex.local:9090` fallaba antes de enviar el Manifest a GitHub.

# Soluciones implementadas
Se amplió únicamente la excepción HTTP local para callbacks Manifest/OAuth y se añadieron pruebas positivas para `dex.local`, `home.arpa`, loopback e IP privada, además de pruebas negativas para HTTP público.

# Pendientes
Ninguno dentro de la librería para este ajuste. Dex debe consumir la versión patch publicada.

# Próximos pasos
Publicar `v0.1.1` y actualizar/desplegar Dex con esa dependencia.
