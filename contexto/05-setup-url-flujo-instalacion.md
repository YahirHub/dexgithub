# Fecha
2026-09-15

# Objetivo
Completar el flujo de instalación de una GitHub App permitiendo configurar `setup_url` desde el Manifest.

# Decisiones tomadas
- `ManifestOptions` incorpora `SetupURL` y `SetupOnUpdate`.
- `SetupURL` usa la misma política segura de callbacks: HTTPS público o HTTP sólo en host local/LAN confiable.
- `SetupURL` no puede combinarse con `RequestOAuth`, de acuerdo con el flujo soportado por GitHub.
- `setup_url` y `setup_on_update` sólo se serializan cuando `SetupURL` está presente.

# Arquitectura actual
El consumidor puede registrar una App con `redirect_url` para completar el Manifest y una `setup_url` separada para volver al panel después de instalar la App y seleccionar repositorios.

# Librerías usadas
Sólo Go standard library.

# Archivos importantes modificados
- `manifest.go`
- `github_test.go`
- `README.md`
- `tareas/completado-04-completar-flujo-instalacion-github.md`

# Problemas encontrados
El flujo anterior terminaba el registro de la App pero no configuraba un retorno automático después de la instalación/permisos.

# Soluciones implementadas
Se añadió soporte explícito para `setup_url` y `setup_on_update` con validación y pruebas.

# Pendientes
Dex debe consumir la versión patch y enlazar su callback de instalación.

# Próximos pasos
Publicar `v0.1.3`, actualizar Dex y validar el flujo Crear App → instalar/elegir repos → volver a Dex.
