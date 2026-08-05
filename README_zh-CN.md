# 自动扩缩容器

<!-- hy-mt2-i18n:start -->
[English](./README.md) | **中文** | [日本語](./README_ja.md) | [Español](./README_es.md)
<!-- hy-mt2-i18n:end -->


根据当前负载自动将 Woodpecker 代理的数量扩展到极致，甚至更高。

## 使用方法

如果您使用的是 docker-compose，可以在您的 `docker-compose.yml` 文件中添加以下内容：

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
      - WOODPECKER_SERVER=https://your-woodpecker-server.tld # 你的 Woodpecker 服务器的地址，也可以是公共地址
      - WOODPECKER_TOKEN=${WOODPECKER_TOKEN} # 可从界面 https://your-woodpecker-server.tld/user/cli-and-api 获取的个人访问令牌
      - WOODPECKER_MIN_AGENTS=0
      - WOODPECKER_MAX_AGENTS=3
      - WOODPECKER_WORKFLOWS_PER_AGENT=2 # 每个代理同时可运行的工作流数量
      - WOODPECKER_GRPC_ADDR=https://grpc.your-woodpecker-server.tld # 你的 Woodpecker 服务器的 gRPC 地址，需能被新创建的代理访问
      - WOODPECKER_GRPC_SECURE=true
      - WOODPECKER_AGENT_ENV= # 可选，用于传递给代理的环境变量
      - WOODPECKER_PROVIDER=hetznercloud # 设置服务提供商，所有可用选项见下方列表
      - WOODPECKER_HETZNERCLOUD_API_TOKEN=${WOODPECKER_HETZNERCLOUD_API_TOKEN} # 用于 Hetzner 云的 API 令牌
```

代理程序将会使用 `WOODPECKER_GRPC_ADDR` 以及自动扩缩容功能在服务器上自动生成的代理令牌来连接服务器。因此，该 `WOODPECKER_GRPC_ADDR` 必须能够被新创建的代理程序从外部访问。例如，您可以参考如何使用 [caddy](https://woodpecker-ci.org/docs/administration/configuration/server#caddy) 来开放 grpc 连接。

## Equinix Metal

将 `WOODPECKER_PROVIDER=equinixmetal` 设定，并至少配置以下内容：

- `WOODPECKER_EQUINIXMETAL_API_TOKEN`
- `WOODPECKER_EQUINIXMETAL_PROJECT_ID`
- `WOODPECKER_EQUINIXMETAL_PLAN`
- `WOODPECKER_EQUINIXMETAL_METRO` 或 `WOODPECKER_EQUINIXMETAL_FACILITY` 中的其中一个

Equinix Metal 支持目前仍处于实验阶段：由于项目维护者均无实际的供应商访问权限，因此尚未经过他们的测试。

有用的可选设置：

- `WOODPECKER_EQUINIXMETAL_OPERATING_SYSTEM`（默认值：`ubuntu_24_04`）
- `WOODPECKER_EQUINIXMETAL_BILLING_CYCLE`（默认值：`hourly`）
- `WOODPECKER_EQUINIXMETAL_TAGS`
- `WOODPECKER_EQUINIXMETAL_PROJECT_SSH_KEYS`
- `WOODPECKER_EQUINIXMETAL_SPOT_INSTANCE`
- `WOODPECKER_EQUINIXMETAL_SPOT_PRICE_MAX`

## OpenStack

将 `WOODPECKER_PROVIDER` 设置为 `openstack`。所有后续环境变量的前缀均为 `WOODPECKER_OPENSTACK_`。

您需要提供指向 Keystone 的 `AUTH_URL`。如有必要，还可以指定 `DOMAIN_NAME`、`REGION` 和 `PROJECT_NAME`。

支持通过 `USERNAME`/`PASSWORD` 进行身份验证，也支持通过 `APPLICATION_CREDENTIAL_ID` 和 `APPLICATION_CREDENTIAL_SECRET` 提供应用凭证。此外，凭证还可以从文件中读取，为此只需在相应的变量名称后加上 `_FILE`，并将其设置为文件路径即可。

您可以通过 `FLAVOR/IMAGE_NAME` 或 UUID 引用（`FLAVOR/IMAGE_REF`）来选择代理实例的规格和镜像。
如果设置了 `VOLUME_SIZE`，则会使用块存储卷。

您可以通过 `KEYPAIR` 参数添加自己的 OpenStack SSH 密钥对。

## 销毁策略

空闲代理的终止方式取决于所选服务提供商的计费模式：

- **按秒计费**（例如 AWS、Scaleway）：当空闲代理处于空闲状态达到 `WOODPECKER_AGENT_IDLE_TIMEOUT` 时间后，就会被终止并移除。保持空闲代理持续运行并无任何意义。
- **按小时向上取整计费**（例如 Linode、Hetzner Cloud、Vultr）：不足一小时的时长也会按完整一小时收费，因此空闲代理会在已付费的剩余时间内继续可调度，直到下一个计费时段开始前才会被终止（该计时起点为其创建时间）。正在运行的代理会直接计入下一个计费小时，无需为空闲时段支付费用。

  代理终止的时间窗口为 `WOODPECKER_AGENT_BILLING_TEARDOWN_MARGIN`（默认值为 `2m`）加上 `WOODPECKER_RECONCILIATION_INTERVAL`，因此对账操作绝不会刚好在时间节点之后执行。按照默认设置（`2m` 的缓冲时间与 `1m` 的间隔时间），空闲代理会在每个计费小时的最后3分钟内达到可终止条件。

计费模式由服务提供商自动选定，因此无需进行任何额外配置即可享受该功能。

## 发展路线图

- [ ] 增加对多个提供商的支持
  - [x] Hetzner Cloud
  - [x] Amazon AWS
  - [ ] Google Cloud
  - [ ] Azure
  - [ ] Digital Ocean
  - [x] Linode
  - [x] OpenStack **[实验性功能]**
  - [ ] Oracle Cloud
  - [x] Equinix Metal **[实验性功能]**（维护者尚未针对真实提供商访问环境进行测试，详见[上文](#equinix-metal)）
  - [x] Vultr
  - [x] Scaleway
- [ ] 清理代理程序
  - [x] 移除存在于提供商端但未列入服务器列表的代理程序（由于没有对应的代理令牌，它们无论如何都无法连接到服务器）
  - [x] 移除供应商端不存在的服务器列表中的代理程序
  - [ ] 移除长时间未连接的代理程序
- [x] 以容器镜像形式发布
- [x] 添加文档
- [ ] 支持根据特定属性（如平台、架构等）部署代理程序
