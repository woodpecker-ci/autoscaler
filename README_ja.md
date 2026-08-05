# 自動スケーラー

<!-- hy-mt2-i18n:start -->
[English](./README.md) | [中文](./README_zh-CN.md) | **日本語** | [Español](./README_es.md)
<!-- hy-mt2-i18n:end -->


現在の負荷に応じて、Woodpeckerエージェントを自動的に無制限にスケーリングできます。

## 使用方法

docker-composeを使用している場合は、`docker-compose.yml`ファイルに以下の内容を追加できます：

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
      - WOODPECKER_SERVER=https://your-woodpecker-server.tld # woodpeckerサーバーのURL。公開可能なURLでも構いません
      - WOODPECKER_TOKEN=${WOODPECKER_TOKEN} # UI https://your-woodpecker-server.tld/user/cli-and-apiから取得できるPersonal Access Token
      - WOODPECKER_MIN_AGENTS=0
      - WOODPECKER_MAX_AGENTS=3
      - WOODPECKER_WORKFLOWS_PER_AGENT=2 # 各エージェントが同時に実行できるワークフローの数
      - WOODPECKER_GRPC_ADDR=https://grpc.your-woodpecker-server.tld # エージェントから公開アクセス可能なwoodpeckerサーバーのgRPCアドレス
      - WOODPECKER_GRPC_SECURE=true
      - WOODPECKER_AGENT_ENV= # エージェントに渡すオプションの環境変数
      - WOODPECKER_PROVIDER=hetznercloud # プロバイダを設定します。利用可能なプロバイダは下記を参照してください
      - WOODPECKER_HETZNERCLOUD_API_TOKEN=${WOODPECKER_HETZNERCLOUD_API_TOKEN} # Hetzner Cloud用のAPIトークン
```

エージェントは、サーバーに接続するために `WOODPECKER_GRPC_ADDR` およびオートスケーラーによってサーバー上で自動的に生成されるエージェントトークンを使用します。そのため、`WOODPECKER_GRPC_ADDR` は新しく作成されるエージェントからも公開的にアクセス可能でなければなりません。例えば、[caddy](https://woodpecker-ci.org/docs/administration/configuration/server#caddy) を使って gRPC 接続を公開する方法を確認してみてください。

## Equinix Metal

`WOODPECKER_PROVIDER=equinixmetal` を設定し、少なくとも以下を構成してください：

- `WOODPECKER_EQUINIXMETAL_API_TOKEN`
- `WOODPECKER_EQUINIXMETAL_PROJECT_ID`
- `WOODPECKER_EQUINIXMETAL_PLAN`
- `WOODPECKER_EQUINIXMETAL_METRO` または `WOODPECKER_EQUINIXMETAL_FACILITY` のうち正確に1つ

Equinix Metalのサポートは現在試験的な段階にあります。プロジェクトのメンテナーたちは実際のプロバイダーアクセス権を持っていないため、テストが行われていません。

便利なオプション設定：

- `WOODPECKER_EQUINIXMETAL_OPERATING_SYSTEM`（デフォルト：`ubuntu_24_04`）
- `WOODPECKER_EQUINIXMETAL_BILLING_CYCLE`（デフォルト：`hourly`）
- `WOODPECKER_EQUINIXMETAL_TAGS`
- `WOODPECKER_EQUINIXMETAL_PROJECT_SSH_KEYS`
- `WOODPECKER_EQUINIXMETAL_SPOT_INSTANCE`
- `WOODPECKER_EQUINIXMETAL_SPOT_PRICE_MAX`

## OpenStack

`WOODPECKER_PROVIDER=openstack` と設定してください。以下のすべての環境変数のプレフィックスは `WOODPECKER_OPENSTACK_` です。

Keystoneを指す`AUTH_URL`を指定する必要があります。必要に応じて、`DOMAIN_NAME`、`REGION`、`PROJECT_NAME`も指定できます。

`USERNAME`/`PASSWORD`による認証と、`APPLICATION_CREDENTIAL_ID`および`APPLICATION_CREDENTIAL_SECRET`を通じたアプリケーション資格情報の両方がサポートされています。
また、資格情報はファイルからも読み込むことができ、その場合は該当する変数名に `_FILE` を付けてファイルパスを設定します。

エージェントインスタンスのフレーバーおよびイメージは、`FLAVOR/IMAGE_NAME` または UUID 参照（`FLAVOR/IMAGE_REF`）を使って指定できます。
`VOLUME_SIZE` を設定すると、ブロックストレージボリュームが使用されます。

`KEYPAIR` を使用して、OpenStack の SSH キーペアを追加できます。

## 廃止ポリシー

アイドル状態のエージェントがいつ削除されるかは、選択したプロバイダーの課金方式によって決まります。

- **秒単位課金**（例: AWS、Scaleway）: アイドル状態が`WOODPECKER_AGENT_IDLE_TIMEOUT`間続くと、そのエージェントは削除されます。アイドルなエージェントを維持しても何のメリットもありません。
- **1時間単位で切り上げる課金**（例: Linode、Hetzner Cloud、Vultr）: 満たない1時間でも1時間分と同じ料金がかかるため、既に支払われた時間の残り期間中は引き続きスケジュール可能な状態が保たれ、次の時間帯の開始直前（作成時刻を基準に）にのみ削除されます。アクティブなエージェントはそのまま次の支払い対象時間帯に移行し、アイドル状態の時間には料金は発生しません。

  アジャスト可能な時間帯は、`WOODPECKER_AGENT_BILLING_TEARDOWN_MARGIN`（デフォルトは`2m`）に`WOODPECKER_RECONCILIATION_INTERVAL`を加えた値となるため、レコンシリエーション処理がその期限をすぐに過ぎてしまうことはありません。デフォルト設定（マージン`2m`、間隔`1m`）の場合、アイドル状態のエージェントは各課金時間の最後の3分間で削除対象となります。

請求モデルはプロバイダーによって自動的に選択されるため、これを利用するための追加設定は一切必要ありません。

## ロードマップ

- [ ] 複数のプロバイダーへの対応追加
  - [x] Hetzner Cloud
  - [x] Amazon AWS
  - [ ] Google Cloud
  - [ ] Azure
  - [ ] Digital Ocean
  - [x] Linode
  - [x] OpenStack **[実験的]**
  - [ ] Oracle Cloud
  - [x] Equinix Metal **[実験的]**（メンテナーによる実際のプロバイダーアクセスでのテストは行われていない。詳細は[上記](#equinix-metal)を参照）
  - [x] Vultr
  - [x] Scaleway
- [ ] エージェントの整理
  - [x] プロバイダー上に存在するがサーバーリストに含まれていないエージェントの削除（エージェントトークンがないため、どうせサーバーに接続できない）
  - [x] プロバイダー上に存在しないサーバーリスト内のエージェントの削除
  - [ ] 長期間接続していないエージェントの削除
- [x] コンテナイメージとしてリリース
- [x] ドキュメントの追加
- [ ] 特定の属性（プラットフォーム、アーキテクチャなど）を持つエージェントのデプロイメントサポート
