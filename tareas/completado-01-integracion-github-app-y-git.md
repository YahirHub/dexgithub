# Tarea 01 — Integración GitHub App y operaciones Git

## Objetivo
Construir una librería Go reutilizable para conectar una aplicación web con GitHub mediante GitHub App, descubrir instalaciones/repositorios accesibles y operar repositorios Git locales/clonados sin persistir tokens en URLs o configuración Git.

## Alcance
- Flujo GitHub App Manifest para crear una app preconfigurada.
- Intercambio del código de manifest por credenciales de la GitHub App.
- JWT de aplicación y tokens de instalación de corta duración.
- OAuth de usuario para validar identidad e instalaciones accesibles.
- Listado de instalaciones y repositorios públicos/privados autorizados.
- Clonado/fetch de repositorios mediante HTTPS sin incrustar tokens en remotes.
- Registro de repositorios locales.
- Root configurable de clones, default `/root/.github/repos`.
- Estado Git, ramas, cambio de rama y lectura de commits.
- Persistencia local segura (0600/0700) de configuración/credenciales/registro.
- Tests unitarios y de integración local sin depender de credenciales reales.
- README y ejemplos de integración.

## Estado
En proceso.
