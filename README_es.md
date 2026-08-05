# Escalador automático

<!-- hy-mt2-i18n:start -->
[English](./README.md) | [中文](./README_zh-CN.md) | [日本語](./README_ja.md) | **Español**
<!-- hy-mt2-i18n:end -->


Ajuste automáticamente la cantidad de agentes Woodpecker según la carga actual, hasta los límites más extremos.

## Uso

Si está utilizando docker-compose, puede agregar lo siguiente a su archivo `docker-compose.yml`:

```yml
# docker-compose.yml
version: '3'

services:
  woodpecker-server:
    image: woodpeckerci/woodpecker-server:next
    [...]

  woodpecker-autoscaler:
    image: woodpeckerci/autoscaler:next
    restart: always
    depends_on:
      - woodpecker-server
    environment:
      - WOODPECKER_SERVER=https://your-woodpecker-server.tld # la URL de tu servidor Woodpecker / también puede ser una URL pública
      - WOODPECKER_TOKEN=${WOODPECKER_TOKEN} # el token de acceso personal que puedes obtener desde la interfaz https://your-woodpecker-server.tld/user/cli-and-api
      - WOODPECKER_MIN_AGENTS=0
      - WOODPECKER_MAX_AGENTS=3
      - WOODPECKER_WORKFLOWS_PER_AGENT=2 # la cantidad de flujos de trabajo que cada agente puede ejecutar al mismo tiempo
      - WOODPECKER_GRPC_ADDR=https://grpc.your-woodpecker-server.tld # la dirección GRPC de tu servidor Woodpecker, accesible públicamente desde los agentes
      - WOODPECKER_GRPC_SECURE=true
      - WOODPECKER_AGENT_ENV= # variables de entorno opcionales para pasar a los agentes
      - WOODPECKER_PROVIDER=hetznercloud # establece el proveedor; puedes encontrar todos los disponibles más abajo
      - WOODPECKER_HETZNERCLOUD_API_TOKEN=${WOODPECKER_HETZNERCLOUD_API_TOKEN} # tu token API para la nube Hetzner
```

Los agentes utilizarán `WOODPECKER_GRPC_ADDR` junto con un token de agente creado automáticamente en el servidor por el autoscaler para conectarse a él. Por lo tanto, `WOODPECKER_GRPC_ADDR` debe ser accesible públicamente desde los agentes recién creados. Consulte, por ejemplo, cómo puede usar [caddy](https://woodpecker-ci.org/docs/administration/configuration/server#caddy) para exponer la conexión grpc.

## Equinix Metal

Establezca `WOODPECKER_PROVIDER=equinixmetal` y configure al menos:

- `WOODPECKER_EQUINIXMETAL_API_TOKEN`
- `WOODPECKER_EQUINIXMETAL_PROJECT_ID`
- `WOODPECKER_EQUINIXMETAL_PLAN`
- exactamente uno de `WOODPECKER_EQUINIXMETAL_METRO` o `WOODPECKER_EQUINIXMETAL_FACILITY`

El soporte para Equinix Metal es actualmente experimental: los mantenedores del proyecto no lo han probado, ya que ninguno de ellos cuenta con acceso real al proveedor.

Configuraciones opcionales útiles:

- `WOODPECKER_EQUINIXMETAL_OPERATING_SYSTEM` (valor predeterminado: `ubuntu_24_04`)
- `WOODPECKER_EQUINIXMETAL_BILLING_CYCLE` (valor predeterminado: `hourly`)
- `WOODPECKER_EQUINIXMETAL_TAGS`
- `WOODPECKER_EQUINIXMETAL_PROJECT_SSH_KEYS`
- `WOODPECKER_EQUINIXMETAL_SPOT_INSTANCE`
- `WOODPECKER_EQUINIXMETAL_SPOT_PRICE_MAX`

## OpenStack

Establezca `WOODPECKER_PROVIDER=openstack`. El prefijo de todas las variables de entorno siguientes es `WOODPECKER_OPENSTACK_`.

Debe proporcionar la variable `AUTH_URL` que apunte a su Keystone. Si es necesario, también puede especificar `DOMAIN_NAME`, `REGION` y `PROJECT_NAME`.

Se admiten tanto la autenticación con `USERNAME`/`PASSWORD` como las credenciales de la aplicación a través de `APPLICATION_CREDENTIAL_ID` y `APPLICATION_CREDENTIAL_SECRET`.  
Las credenciales también se pueden leer desde archivos; para ello, añada `_FILE` al nombre de la variable correspondiente e indique la ruta del archivo.

Puede seleccionar el tipo y la imagen para las instancias de agente a través de `FLAVOR/IMAGE_NAME` o mediante una referencia UUID (`FLAVOR/IMAGE_REF`).  
Si establece `VOLUME_SIZE`, se utilizarán volúmenes de almacenamiento en bloque.

Puede agregar su par de claves SSH de OpenStack mediante `KEYPAIR`.

## Política de desmantelamiento

La forma en que se eliminan los agentes inactivos depende de cómo cobra el proveedor seleccionado:

- **Facturación por segundo** (p. ej., AWS, Scaleway): un agente inactivo se detiene y elimina una vez que ha permanecido inactivo durante `WOODPECKER_AGENT_IDLE_TIMEOUT`. Mantener un agente inactivo activo no aporta ningún beneficio.
- **Facturación redondeada por hora** (p. ej., Linode, Hetzner Cloud, Vultr): una hora parcial cuesta lo mismo que una hora completa, por lo que un agente inactivo sigue estando disponible para programaciones durante el resto de la hora ya pagada, y solo se detiene justo antes del inicio de la siguiente hora (con base en su tiempo de creación). Un agente ocupado simplemente pasa a formar parte de la siguiente hora de pago; nunca se paga por una hora en la que está inactivo.

  El período de desmantelamiento es `WOODPECKER_AGENT_BILLING_TEARDOWN_MARGIN` (valor por defecto: `2m`) más `WOODPECKER_RECONCILIATION_INTERVAL`; por lo tanto, la reconciliación nunca puede superar ese límite directamente. Con los valores predeterminados (margen de `2m` e intervalo de `1m`), un agente inactivo pasa a ser apto para su desmantelamiento durante los últimos 3 minutos de cada hora de pago.

El proveedor selecciona automáticamente el modelo de facturación, por lo que no se requiere ninguna configuración adicional para beneficiarse de ello.

## Hoja de ruta

- [ ] Agregar soporte para múltiples proveedores
  - [x] Hetzner Cloud
  - [x] Amazon AWS
  - [ ] Google Cloud
  - [ ] Azure
  - [ ] Digital Ocean
  - [x] Linode
  - [x] OpenStack **[experimental]**
  - [ ] Oracle Cloud
  - [x] Equinix Metal **[experimental]** (no ha sido probado por los mantenedores con acceso real al proveedor; véase [arriba](#equinix-metal))
  - [x] Vultr
  - [x] Scaleway
- [ ] Limpieza de agentes
  - [x] Eliminar agentes que existen en el proveedor pero no figuran en la lista de servidores (de todos modos no podrán conectarse al servidor ya que no cuentan con token de agente)
  - [x] Eliminar de la lista de servidores a aquellos agentes que no existen en el proveedor
  - [ ] Eliminar agentes que han estado desconectados durante mucho tiempo
- [x] Publicar como imagen de contenedor
- [x] Agregar documentación
- [ ] Soportar el despliegue de agentes con atributos específicos (p. ej., plataformas, arquitecturas, etc.)
