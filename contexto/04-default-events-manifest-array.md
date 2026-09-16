# Fecha
2026-09-15

# Objetivo
Corregir el GitHub App Manifest generado cuando no se solicitan eventos webhook explícitos.

# Decisiones tomadas
- GitHub exige que `default_events` sea un arreglo JSON.
- Cuando `ManifestOptions.Events` sea `nil`, DexGitHub serializa `default_events` como `[]`, no como `null`.
- No se agregan eventos por defecto: el arreglo vacío conserva el principio de mínimos permisos/eventos.

# Arquitectura actual
La normalización ocurre dentro de `Service.ManifestForm` justo antes de construir el mapa del manifest.

# Librerías usadas
Sólo Go standard library.

# Archivos importantes modificados
- `manifest.go`
- `github_test.go`
- `tareas/completado-03-corregir-default-events-manifest.md`

# Problemas encontrados
GitHub rechazó el manifest con: `For 'properties/default_events', nil is not an array.` porque una slice `nil` se serializaba como JSON `null`.

# Soluciones implementadas
Una slice de eventos ausente se normaliza a `[]string{}` y la prueba de regresión deserializa el JSON para verificar que `default_events` sea un arreglo vacío.

# Pendientes
Ninguno dentro de la librería para este ajuste. Dex debe consumir la versión patch publicada.

# Próximos pasos
Publicar `v0.1.2`, consumirla desde Dex y desplegar el binario actualizado.
