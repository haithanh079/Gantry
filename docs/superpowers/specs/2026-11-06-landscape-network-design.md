# Gantry Landscape: model triển khai, Observed Inventory và Network Path

- **Ngày:** 2026-11-06
- **Trạng thái:** Draft v3 (thêm DataFlow, Highlight, nhập tay Cloudflare qua MCP, tag allowlist cấu hình được), chờ user review
- **Phạm vi:** Sub-project 1 + 2 + 3 (model dữ liệu, Landscape view, network path A → B, luồng dữ liệu / job)
- **Ngoài phạm vi (spec riêng sau):** tech stack / version / outdated / CVE (sub-project 4); discovery qua cloud API AWS/GCP/Cloudflare (sub-project 5). Cloudflare API không có ở v1; dữ liệu Cloudflare đến từ Terraform hoặc nhập tay qua MCP (mục 6.3)
- **Mockup UI:** Open Design project `gantry-landscape`
- **Triển khai:** chia 3 implementation plan độc lập, theo thứ tự A → B → C (mục 15)

## 1. Bối cảnh và vấn đề

Công ty có nhiều phòng ban; mỗi phòng ban có nhiều game mobile và nhiều backend service. Mỗi service chạy ở nhiều env (dev, qc, staging, production), trên nhiều cloud (GCP, AWS, Cloudflare) với nhiều runtime (Cloud Run, GKE, EKS, ECS, EC2, CF Workers) và nhiều loại DB (Cloud SQL, RDS Postgres, Aurora, DynamoDB, Neptune). Network multi-region, PrivateLink ra service ngoài, kết nối cross-cloud.

Architect và người vận hành cloud không có bức tranh tổng thể: không filter/zoom được theo team, env, project, cloud; không trả lời được "A nói chuyện với B qua đường nào, có thông không".

### Use case (theo thứ tự ưu tiên)

1. **Kiểm kê:** phòng ban / game X có gì, chạy ở cloud / region / runtime nào.
2. **Network:** A đến B qua đường nào (peering, PrivateLink, VPN, public), có thực sự reachable không.
3. **Review kiến trúc:** prod gọi qc, chỉ đi qua public internet, thiếu owner, dependency khai báo nhưng bị chặn.
4. **Blast radius:** DB / link / VPC Y chết thì deployment, game, phòng ban nào bị ảnh hưởng.
5. **Drift:** khai báo khác thực tế.
6. **Luồng dữ liệu / job:** thấy được các luồng không phải lời gọi service, ví dụ job copy dữ liệu từ Cloudflare R2 sang S3: ai chạy, chạy lúc nào, dữ liệu loại gì, đi qua đường nào; đánh dấu và highlight các luồng / nhóm node quan trọng trên map.

### Ràng buộc đã chốt

| Mục | Quyết định |
|---|---|
| Quy mô | 100–500 deployment (service × env) |
| Độ sâu network | Đầy đủ: VPC, subnet, NAT/IGW, SG, firewall rule, route table, đủ để tính reachability thật |
| Tổ chức | Department (phòng ban) ⊃ Team; Project (game) many-to-many Service; có shared platform service. Owner và game sử dụng là hai trục độc lập |
| Nguồn dữ liệu | Hybrid: catalog khai báo (YAML/GitOps, UI, MCP) là nền; importer bổ sung và đối chiếu |
| Quyền truy cập cloud | Giai đoạn 1: chỉ Terraform state, được đẩy vào bởi agent nội bộ (Gantry không giữ cloud credential). Phần Terraform không quản lý (tunnel, DNS, …) được nhập tay qua MCP. Cloud API (kể cả Cloudflare) sau |
| Kiến trúc | Catalog + lớp Observed Inventory tách biệt |

## 2. Gap analysis so với Gantry hiện tại

- Built-in kinds (`internal/entity/kinds.go:18`): `Service`, `API`, `Infrastructure`, `Team`, `Environment`, `Documentation`, `Flow`, `Action`; schema đều `additionalProperties: false`.
- Quan hệ chỉ có `dependsOn`, `deployedIn`, `providesApis`, `consumesApis`, owner; edge không có thuộc tính.
- Entity ref chỉ có `{kind, name}`. Nhiều chỗ resolve theo kind/name hoặc giả định cùng namespace: `internal/db/queries.go` (`GetEntityGraph`, edge id `kind/name`), `internal/api/handlers/topology.go` (match env theo name), `internal/mcp/service_tools.go` (`link_service` nhận `kind` + `targetName`). Dữ liệu thật đều nằm ở `default`.
- Kind `Flow` hiện có là sơ đồ vẽ tay (viewport, node, vị trí), không mang ngữ nghĩa luồng dữ liệu; giữ nguyên, không tái sử dụng cho DataFlow.
- Thiếu: cấp phòng ban, trục game, "service chạy ở env cụ thể", runtime platform, cloud account, network, connectivity, engine / version DB, luồng dữ liệu / job, cơ chế đánh dấu trên map; enum env hiện là `staging | production | development` (`internal/entity/schemas/environment.json`).
- RBAC: `CanAccess(resourceType, resourceFilter, action)` chỉ lọc theo kind (`internal/api/handlers/access.go`, `internal/auth/rbac.go`); MCP write tools kiểm tra role level (`requireDeveloperRole`, `internal/mcp/write_tools.go`).
- Đã có bảng `groups` cho RBAC / SSO groups (`internal/db/migrations.go`), nên kind phòng ban không được đặt tên `Group`.
- `TopologyExplorer`: nhóm theo environment, SVG tự vẽ, không filter theo team / cloud / project, không có network layer.
- Plugin ingest hiện có: Kubernetes, GitHub, ArgoCD, Azure. Không có AWS / GCP / Cloudflare / Terraform. Repo công ty ở GitLab.
- GitOps serializer hoạt động generic theo kind (`internal/gitops/serializer.go`), nên kind mới chỉ cần schema + đăng ký trong `BuiltinKinds`.

## 3. Kiến trúc tổng quan

```
             ┌────────────── Catalog (human-owned, GitOps) ──────────────┐
             │ Department · Team · Project · Service · Infrastructure     │
             │ Environment · CloudAccount · Network · Connectivity        │
             │ Deployment (connectsTo) · DataFlow · Highlight           │ │
             │                                            externalRefs ─┐ │
             └──────────────────────────────────────────────────────────┼─┘
                                                                        │ link (declared | auto | suggested)
             ┌──────────── Observed Inventory (machine-owned) ──────────▼─┐
             │ observed_sources · observed_snapshots                       │
             │ observed_resources · observed_relations                     │
             │ entity_observed_links                                       │
             └────────▲────────────────────────────────────────────────────┘
                      │ import (REST gzip upload / MCP)      │ upsert_manual_resources (MCP)
       agent nội bộ đọc Terraform state ──► sanitizer ──► normalizer   người / agent nhập tay (CF tunnel, DNS, R2…)
                      │
                      ▼
      Reachability engine ──► reachability_results ──► Findings / Drift
                      │
                      ▼
      Landscape UI: Map · Network · Inventory · Path · Blast radius
```

Nguyên tắc:
- Catalog giữ ở quy mô con người, vẫn đi qua GitOps / audit / search như hiện nay.
- Observed Inventory không vào GitOps và không vào search chung; audit ở mức snapshot (metadata, không chứa nội dung).
- Observed v1 là **latest-only** (không time-travel). Snapshot chỉ lưu metadata và thống kê diff.
- Mọi kết luận network truy được về bằng chứng (resource id / rule id) và serial của state.

## 4. Model catalog

### 4.1 Ref chuẩn

- Ref có cấu trúc: `{kind, name, namespace?}`. `namespace` thiếu thì dùng namespace của entity chứa ref, rồi fallback `default`.
- **Chuỗi ref canonical** dùng ở mọi nơi (DB columns, edge id, API, tag cloud, UI URL): `Kind/namespace/name`, ví dụ `Deployment/music/nakama-production`.
- Hàm duy nhất `entity.ResolveRef(container *Entity, ref Ref) CanonicalRef` và `entity.ParseCanonicalRef(string)`; mọi call site dùng hàm này.
- Call site phải cập nhật (Plan A): `internal/db/queries.go` (`GetEntityGraph` + edge id), `internal/api/handlers/topology.go`, `internal/api/handlers/graph.go`, `internal/mcp/service_tools.go` (`link_service` thêm `targetNamespace`), `web/src/lib/utils.ts` (`catalogEntityPath`), `web/src/pages/TopologyExplorer.tsx` (`topologyEntityPath`), `web/src/components/EntityGraph.tsx`.
- Ref không resolve được **không chặn ghi** (để GitOps import không phụ thuộc thứ tự); thay vào đó sinh finding `dangling-ref` (mục 9).

### 4.2 Namespace

Một namespace cho mỗi Department (slug của Department); `platform` cho shared service. Dữ liệu cũ ở `default` vẫn hợp lệ.

### 4.3 Kind mới

**`Department`**: phòng ban / business unit / platform (flat, không lồng nhau ở v1).

| Field | Kiểu |
|---|---|
| `type` | enum `department \| business-unit \| platform` |
| `leads` | string[] |
| `slackChannel` | string |
| `links` | như Team |

**`Project`**: game / sản phẩm.

| Field | Kiểu |
|---|---|
| `type` | enum `game \| platform \| internal-tool` |
| `department` | Ref Department |
| `platforms` | enum[] `ios \| android \| web \| desktop` |
| `status` | enum `incubating \| live \| sunset` |
| `links` | |

**`CloudAccount`**: AWS account / GCP project / Cloudflare account.

| Field | Kiểu |
|---|---|
| `provider` | enum `aws \| gcp \| cloudflare` |
| `accountId` | string |
| `department` | Ref Department |
| `envType` | enum `development \| qc \| staging \| production \| sandbox \| shared` |
| `externalRefs` | ExternalRef[] |

**`Network`**

| Field | Kiểu | Ghi chú |
|---|---|---|
| `provider` | enum `aws \| gcp \| cloudflare \| external \| on-prem` | |
| `networkType` | enum `vpc \| shared-vpc \| transit-hub \| edge \| external` | `transit-hub` = AWS TGW / GCP NCC hub; `edge` = Cloudflare account edge; `external` = SaaS / đối tác |
| `cloudAccount` | Ref CloudAccount | bỏ trống nếu `external` |
| `region` | string | GCP VPC dùng `global` |
| `cidrs` | string[] | |
| `externalRefs` | ExternalRef[] | |

**`Connectivity`**: kết nối giữa hai Network.

| Field | Kiểu | Ghi chú |
|---|---|---|
| `type` | enum `vpc-peering \| tgw-attachment \| privatelink \| psc \| vpn \| interconnect \| direct-connect \| cf-tunnel \| public-internet` | TGW được mô hình là Network `transit-hub` + mỗi VPC một `tgw-attachment`; route table TGW nằm ở lớp observed |
| `from`, `to` | Ref Network | PrivateLink / PSC: `from` = consumer, `to` = producer |
| `bidirectional` | bool | PrivateLink / PSC luôn `false` |
| `externalRefs` | ExternalRef[] | |

**`Deployment`**: một instance của Service hoặc Infrastructure trong một Environment.

| Field | Kiểu | Ghi chú |
|---|---|---|
| `target` | Ref Service \| Infrastructure | bắt buộc |
| `environment` | Ref Environment | bắt buộc |
| `cloudAccount` | Ref CloudAccount | |
| `regions` | string[] | |
| `network` | Ref Network | |
| `runtime.platform` | enum `cloud-run \| gke \| eks \| ecs \| ec2 \| lambda \| cf-workers \| managed \| docker` | `managed` cho DB / queue managed |
| `runtime.cluster` | string | |
| `runtime.workload` | string | ECS service / k8s workload / Cloud Run service; dùng để auto-link |
| `ingress[]` | `{type: alb\|nlb\|cf\|apigw\|cloud-lb, host, visibility: public\|internal}` | |
| `version` | string | |
| `connectsTo[]` | `{deployment: Ref, protocol: tcp\|udp, port: int, note}` | |
| `externalRefs` | ExternalRef[] | |

Quy ước tên: `<target>-<env>` (ví dụ `nakama-production`), unique trong namespace.

**`DataFlow`**: luồng dữ liệu / job giữa hai điểm, không phải lời gọi service. Ví dụ: job copy object từ R2 sang S3.

| Field | Kiểu | Ghi chú |
|---|---|---|
| `source`, `target` | Ref Deployment \| Infrastructure | bắt buộc; Infrastructure chưa có Deployment vẫn dùng được (ví dụ bucket R2 của đối tác) |
| `actor` | Ref Deployment | job / worker thực thi (cron trên ECS, Lambda, CF Worker, Cloud Run job…); bỏ trống nếu là tính năng managed (ví dụ S3 replication) |
| `mode` | enum `batch \| stream \| replication \| event \| sync \| export \| backup` | |
| `schedule` | string | cron hoặc mô tả tự do |
| `transport` | enum `public-internet \| private \| privatelink \| vpc-endpoint \| cf-tunnel \| provider-internal \| unknown` | khai báo của con người; Plan C đối chiếu với reachability |
| `dataClasses` | string[] | tự do, gợi ý `pii`, `payment`, `telemetry`, `assets`, `backup`, `logs` |
| `direction` | enum `push \| pull` | actor đẩy sang target hay kéo từ source; chỉ để hiển thị |
| `volumeHint` | string | ví dụ `~200 GB/ngày` |
| `status` | enum `active \| paused \| deprecated` | |
| `links` | như Team | runbook, dashboard, repo |

Owner dùng `spec.owner` như các kind khác. DataFlow là entity nên đi qua GitOps / audit / search như bình thường, và tham gia blast radius (mục 10): một trong `source` / `target` / `actor` hỏng thì luồng bị ảnh hưởng.

**`Highlight`**: lớp đánh dấu có tên, dùng để nổi bật một tập node / cạnh trên map. Ví dụ "Cross-cloud data egress", "Sự cố 2026-11-05", "PCI scope".

| Field | Kiểu | Ghi chú |
|---|---|---|
| `color` | enum `mark-1 … mark-6` | map sang biến `--gantry-mark-1..6` (mục 11) |
| `members[]` | Ref (mọi kind: Deployment, Service, DataFlow, Connectivity, Network, …) | chọn tường minh |
| `edges[]` | `{from: Ref Deployment, to: Ref Deployment}` | cạnh `connectsTo` (khai báo hoặc suy luận) |
| `selector` | `{kinds[], environments[], clouds[], projects[], departments[], labels{}, dataClasses[], transports[], modes[]}` | các điều kiện AND giữa field, OR trong một field; `dataClasses` / `transports` / `modes` chỉ áp lên DataFlow |
| `includeNeighbors` | bool | thêm cạnh nối giữa các node đã match (mặc định `true`) |
| `defaultOn` | bool | bật sẵn khi mở Landscape |
| `note` | string (markdown) | |

Tập match = `members` ∪ `edges` ∪ kết quả `selector`. Highlight không đổi dữ liệu nào khác, chỉ là lớp trình bày; quyền xem tuân theo `FilterEntitiesByAccess` (member không xem được thì bị loại khỏi kết quả, không lộ tên).

### 4.4 Sửa kind hiện có

- `Team`: thêm `department`.
- `Service`: thêm `projects` (Ref Project[]). Giữ `system`.
- `Environment`: thêm `qc`, `sandbox` vào `type`. `provider` / `region` / `accountId` / `cluster` đánh dấu deprecated (vẫn đọc, UI ẩn).
- `Infrastructure`: thêm `engine` (`postgres \| aurora-postgres \| aurora-mysql \| mysql \| dynamodb \| neptune \| redis \| kafka \| s3 \| gcs \| r2 \| cf-kv \| cf-d1 \| other`), `managedBy` (`cloud-sql \| rds \| aurora \| dynamodb \| neptune \| msk \| memorystore \| elasticache \| s3 \| gcs \| cloudflare \| self-hosted`), `version`, `externalRefs`; thêm `graph-db` vào `type`.
- Mọi ref field cũ (`dependsOn`, `deployedIn`, …) cho phép thêm `namespace`.

### 4.5 Kiểu dùng chung

```yaml
ExternalRef:
  source: terraform | aws | gcp | cloudflare | manual
  id: string           # ARN / GCP resource name / CF id
  tfAddress: string    # vd: module.ecs.aws_ecs_service.nakama
  stateSource: string  # observed_sources.id
```

### 4.6 Suy luận `connectsTo`

- Với mỗi Service A `dependsOn` B và mỗi Environment E mà A và B đều có Deployment: sinh cạnh suy luận `A@E → B@E`.
- Port mặc định theo `Infrastructure.engine`: postgres / aurora-postgres 5432, mysql / aurora-mysql 3306, neptune 8182, redis 6379, kafka 9092; Service hoặc engine không có mặc định thì port = rỗng (reachability đánh giá ở mức "bất kỳ port nào được mở" và gắn caveat).
- Cạnh khai báo trong `connectsTo` override cạnh suy luận cùng cặp.
- Cạnh cross-env chỉ tồn tại khi khai báo.
- DataFlow **không** sinh cạnh `connectsTo`. Ở Plan C, mỗi DataFlow có actor sinh hai truy vấn reachability ẩn: actor → source và actor → target (port theo engine; R2 / S3 / GCS là 443).

### 4.7 Migration dữ liệu hiện có

- CLI `gantry migrate landscape [--apply]` và MCP tool `migrate_landscape {apply: bool}`; mặc định dry-run, in ra danh sách entity sẽ tạo / sửa.
- Mỗi `deployedIn` của Service / Infrastructure thành một Deployment; `provider` / `region` của Environment chép sang Deployment.
- `system` được đề xuất thành Project (ví dụ `personalize`); chỉ tạo khi `--apply`.
- Catalog hiện tại (đã đếm): nakama 3 (local, staging, production); upe-service, upe-admin, aurora, dynamodb-upe, msk, neptune, s3-app-storage, s3-ecs-env, s3-telemetry mỗi cái 2; postgres-local 1. Tổng **22 Deployment**.

## 5. RBAC

- Resource type mới cho `CanAccess`:
  - `observed` với action `read` / `write` (`write` = import, nhập tay resource, confirm / dismiss link);
  - `landscape` với action `read` (graph, inventory, path, findings).
- Default theo role: viewer đọc được `observed` / `landscape`; developer có thêm `observed:write`.
- Import tool (MCP + REST) kiểm tra bằng `CanAccess(ctx, "observed", <sourceId|*>, "write")`, **không** dùng `requireDeveloperRole`.
- Agent importer chạy dưới một **service user riêng** (ví dụ `svc-infra-importer`) có role `viewer` và một allow rule `observed:write` trong `permission_rules` (rule gắn với user / group; API key hành động thay mặt user sở hữu nó). API key của agent thuộc service user này. `owner_principal` lưu **user id**, không phải key id, để xoay key không mất quyền sở hữu source.
- Phân quyền theo từng CloudAccount / namespace: **ngoài phạm vi v1**. `resourceFilter` = id của observed source để chuẩn bị cho bước sau.

## 6. Observed Inventory

### 6.1 Bảng (migration mới trong `internal/db/migrations.go`)

```sql
observed_sources(
  id TEXT PRIMARY KEY,               -- slug do client đặt, vd "tf-nakama-upe-prod"
  type TEXT NOT NULL,                -- terraform | manual | (sau: aws | gcp | cloudflare)
  locator TEXT NOT NULL,             -- s3://bucket/key | gs://... | gitlab:<project>/<state> | manual:<cloud account ref>
  cloud_account_ref TEXT,            -- canonical ref
  owner_principal TEXT NOT NULL,     -- user id tạo source; với terraform chỉ user này hoặc admin được ghi đè (manual: xem 6.3)
  lineage TEXT, last_serial INTEGER,
  last_import_at TIMESTAMP, status TEXT, error TEXT)

observed_snapshots(
  id TEXT PRIMARY KEY, source_id TEXT NOT NULL, serial INTEGER,
  content_hash TEXT, imported_at TIMESTAMP, imported_by TEXT,
  resource_count INTEGER, added INTEGER, changed INTEGER, removed INTEGER)
  INDEX (source_id, imported_at)

observed_resources(
  id TEXT PRIMARY KEY,               -- sha256(source_id + tf_address) | manual: sha256(source_id + key)
  source_id TEXT NOT NULL,
  provider TEXT, type TEXT, native_type TEXT,
  external_id TEXT, tf_address TEXT, manual_key TEXT, name TEXT,
  account_id TEXT, region TEXT,
  attrs TEXT,                        -- JSON, chỉ allowlisted attributes
  tags TEXT,                         -- JSON, chỉ allowlisted tag keys
  first_seen TIMESTAMP, last_seen TIMESTAMP, deleted_at TIMESTAMP,
  updated_by TEXT)                   -- chỉ dùng cho manual
  INDEX (source_id), INDEX (type), INDEX (external_id)

observed_relations(
  from_id TEXT, to_id TEXT, relation TEXT,
  PRIMARY KEY (from_id, to_id, relation))
  -- relation: in_vpc | in_subnet | has_sg | routes_via | attaches | targets | peers_with | exposes
  --           | runs_in (cf_tunnel → subnet/workload) | routes_to (cf_tunnel_route → tunnel) | resolves_to (cf_dns_record → tunnel/lb/worker)

entity_observed_links(
  entity_ref TEXT, resource_id TEXT,
  state TEXT NOT NULL,               -- declared | auto | suggested | dismissed
  reason TEXT, created_at TIMESTAMP, decided_by TEXT,
  PRIMARY KEY (entity_ref, resource_id))
  INDEX (resource_id)

reachability_results(
  from_ref TEXT, to_ref TEXT, protocol TEXT, port INTEGER,
  input_hash TEXT NOT NULL,          -- xem 8.5
  verdict TEXT NOT NULL,             -- reachable | reachable-with-caveats | blocked | unknown
  result TEXT NOT NULL,              -- JSON Result
  computed_at TIMESTAMP,
  PRIMARY KEY (from_ref, to_ref, protocol, port))
```

Retention: `observed_snapshots` giữ 50 bản / source; resource có `deleted_at` quá 30 ngày thì xóa hẳn.

Đổi địa chỉ Terraform (`moved` block, đổi module): khi một resource biến mất và trong cùng import xuất hiện resource mới có cùng `external_id`, coi là một resource. Dời `entity_observed_links` sang id mới và giữ `first_seen`, để link đã confirm không bị mất.

Bộ đếm `observed_generation` (một dòng trong bảng `settings` hiện có hoặc bảng mới `observed_meta`) tăng sau mỗi import thành công; dùng cho cache reachability (8.5).

### 6.2 Type chuẩn hóa (v1)

`vpc, subnet, route_table, route, igw, nat, tgw, tgw_route_table, tgw_attachment, vpc_peering, vpce, vpce_service, vpn_connection, vgw, interconnect_attachment, security_group, sg_rule, nacl (lưu, chưa đánh giá), gcp_firewall, gcp_firewall_policy, lb, lb_listener, target_group, ecs_cluster, ecs_service, eks_cluster, gke_cluster, cloud_run_service, vpc_access_connector, ec2_instance, lambda, rds_cluster, rds_instance, cloudsql_instance, dynamodb_table, neptune_cluster, msk_cluster, elasticache, memorystore, s3_bucket, gcs_bucket, r2_bucket, cf_tunnel, cf_tunnel_route, cf_worker, cf_dns_record, cf_zone, external_endpoint, generic`.

Attribute chính của nhóm Cloudflare (cũng là allowlist cho nhập tay):
- `cf_tunnel`: `id, name, account_id, status`; relation `runs_in` tới subnet / Deployment nơi chạy `cloudflared`.
- `cf_tunnel_route`: `tunnel_id, hostname, path, service` (ví dụ `http://nakama.internal:7350`), hoặc `network_cidr` cho private network route.
- `cf_dns_record`: `zone, name, type, content, proxied, ttl`.
- `cf_zone`: `id, name`.
- `cf_worker`: `name, routes[], bindings[*].{type,name,target}` (chỉ tên binding, không giá trị secret / env var).
- `r2_bucket`: `name, location, public_access, custom_domains[]`.
- `external_endpoint`: `name, hosts[], cidrs[], ports[]` cho SaaS / đối tác không nằm trong state.

### 6.3 Nhập tay resource (source `manual`, Plan B)

Dùng cho phần Terraform không quản lý (thường là Cloudflare tunnel / DNS / R2 / Workers tạo bằng dashboard) khi chưa có Cloudflare API.

- Một source `manual` cho mỗi CloudAccount, id gợi ý `manual-<account slug>`; tự tạo ở lần ghi đầu tiên.
- MCP `upsert_manual_resources {sourceId, cloudAccount, resources: [{key, type, name, externalId?, region?, attrs, tags, relations: [{relation, toKey? | toResourceId?}]}]}`. `key` do client đặt, ổn định (ví dụ `tunnel/nakama-prod`); cùng `key` là cập nhật.
- MCP `delete_manual_resource {sourceId, key}` đặt `deleted_at` (theo retention 30 ngày như resource khác).
- MCP `list_observed_resources {sourceId?, type?, query?}` để đọc lại (cả terraform lẫn manual).
- Validate:
  - `type` phải thuộc tập cho phép nhập tay: các type Cloudflare ở 6.2, `external_endpoint`, `generic`.
  - `attrs` phải nằm trong allowlist của type; key ngoài allowlist hoặc khớp denylist thì **từ chối cả request** (khác importer: importer lặng lẽ bỏ, nhập tay báo lỗi rõ để người nhập sửa).
  - Tag theo cùng allowlist với importer (6.4).
  - `toKey` phải trỏ tới resource trong cùng source; `toResourceId` có thể trỏ sang source khác (ví dụ tunnel `runs_in` một subnet từ Terraform).
- Quyền: `observed:write`. Source `manual` **không** khóa theo `owner_principal`: mọi principal có quyền đều sửa được; `updated_by` và audit ghi lại người sửa.
- Mỗi lần upsert / delete: một transaction, ghi một `observed_snapshots` (serial tăng dần, counts), một audit entry (key, type, attrs đã validate; vì attrs đã qua allowlist nên được phép vào audit), tăng `observed_generation`, phát event `observed.snapshot.imported`.
- Hiển thị: resource manual có badge "Manual" ở Inspector / Network lens; evidence trong Path ghi rõ nguồn manual và người sửa cuối.
- Khi một source terraform import resource có cùng `external_id` với resource manual: sinh finding `manual-shadowed` (info) gợi ý xóa bản nhập tay; engine ưu tiên bản terraform.

### 6.4 Tag allowlist

- Mặc định: `Name`, `env`, `environment`, `team`, `service`, `project`, `owner`. Luôn giữ `gantry.io/*` (cần cho auto-link) bất kể cấu hình.
- Cấu hình qua plugin config `observed.tags`:
  - `extra: string[]`: thêm key hoặc pattern có `*` (ví dụ `ama:*`, `cost-center`); để trống nếu không cần.
  - `replaceDefaults: bool` (mặc định `false`): `true` thì chỉ dùng `extra` + `gantry.io/*`.
- Không có key nội bộ bắt buộc. Đổi cấu hình chỉ áp dụng cho lần import sau (không xử lý lại dữ liệu cũ vì raw state không được lưu).

## 7. Importer Terraform state (Plan B)

### 7.1 Đường vào

- **REST (khuyến nghị cho state lớn):** `POST /api/v1/observed/import` với body `application/gzip` (tfstate JSON v4 đã gzip), header `X-Gantry-Source-Id`, `X-Gantry-Source-Locator`, `X-Gantry-Cloud-Account`. Giới hạn: 50 MB sau giải nén (`GANTRY_OBSERVED_MAX_STATE_MB`).
- **MCP:** `import_terraform_state {sourceId, locator, cloudAccount, stateGzipBase64}`, giới hạn 10 MB sau giải nén; lớn hơn thì trả lỗi hướng dẫn dùng REST.
- Quyền: `observed:write` (mục 5). Nếu source đã tồn tại thì principal phải là `owner_principal` hoặc admin.

### 7.2 Xử lý

1. Giải nén dạng streaming với `io.LimitReader` theo giới hạn sau giải nén (chống zip bomb), validate `version == 4`, đọc `lineage` / `serial`. Cùng lineage và `serial` ≤ `last_serial` thì no-op (idempotent).
2. **Sanitize** (package `internal/observed/sanitize`, test bắt buộc):
   - Bỏ toàn bộ `outputs` top-level.
   - Bỏ mọi path trong `sensitive_attributes`.
   - **Allowlist là cơ chế chính:** chỉ giữ attribute nằm trong allowlist path của từng type chuẩn hóa (ví dụ `security_group`: `id, arn, name, vpc_id, ingress[*].{from_port,to_port,protocol,cidr_blocks,ipv6_cidr_blocks,prefix_list_ids,security_groups,self}`, `egress[*]…`). Allowlist không bao gồm các block env var / secret của task / container definition, Lambda environment, Cloud Run env, `user_data`.
   - `generic` chỉ giữ `id`, `arn`, `name`, `self_link`.
   - Denylist chỉ là lưới an toàn thứ hai, so **toàn bộ tên key** (exact, không phân biệt hoa thường), không so substring: `password`, `master_password`, `secret`, `secret_string`, `private_key`, `token`, `connection_string`, `credentials`, `kubeconfig`, `client_key`, `user_data`. Các key như `certificate_arn`, `secret_arn` không bị ảnh hưởng.
   - Tags: chỉ giữ key thuộc tag allowlist (6.4).
   - Raw state không được lưu, không log, không đưa vào audit `BeforeState` / `AfterState`, không trả lại trong response hoặc MCP output.
3. Normalizer theo provider (`internal/observed/normalize/{aws,google,cloudflare}.go`) map sang type chuẩn, sinh `observed_relations`.
4. Trong một transaction: upsert resources, đặt `deleted_at` cho resource biến mất, ghi snapshot, cập nhật source.
5. Audit: một entry / import với metadata (source, serial, added / changed / removed). Quy tắc lâu dài: request body của route import không bao giờ được log. Hiện chưa có middleware log body; nếu sau này thêm thì phải loại trừ route này, và có test bảo vệ.
6. Event `observed.snapshot.imported` qua event bus / WebSocket, rồi chạy auto-link (7.3) và xếp job reachability (8.6).
7. Lỗi parse / validate: 400 với mã lỗi và JSON pointer vị trí, không kèm nội dung; `status=error`; không ghi một phần.

### 7.3 Auto-link

Thứ tự ưu tiên:
1. `externalRefs` trên entity, cho state `declared`.
2. Cloud tag `gantry.io/entity=<canonical ref>`, cho state `auto`.
3. Khớp `runtime.workload` (+ `runtime.cluster`) với ECS service / k8s workload / Cloud Run service, hoặc DB identifier khớp tên Deployment / Infrastructure, cho state `suggested`.

MCP tools: `list_link_suggestions`, `confirm_link`, `dismiss_link` (quyền `observed:write`).

## 8. Reachability engine (Plan C)

Package mới `internal/reachability`, Go thuần.

### 8.1 Đầu vào / đầu ra

```
Query{From: CanonicalRef (Deployment); To: CanonicalRef (Deployment, hoặc Infrastructure có engine object storage: s3 | gcs | r2); Protocol: tcp|udp; Port: int (0 = any)}
Result{Verdict; Caveats []string; Paths []Path; InputHash string}
Path{Kind: private|public; Verdict: reachable|blocked|unknown; Hops []Hop}
Hop{Kind; ResourceID; Label; Direction: forward|return; Verdict: pass|fail|unknown; Evidence []Evidence}
Evidence{ResourceID; NativeID; Rule string; Explanation string}
```

### 8.2 Resolve endpoint

- **Nguồn** (tập địa chỉ + identity của workload A):
  - ECS / EC2 / Lambda-in-VPC: subnets + SGs của task / instance / function.
  - EKS: subnets + SG của node / pod (security groups for pods nếu có).
  - GKE: subnet + secondary range pod (alias IP), network tags / service account của node pool.
  - Cloud Run: egress qua `vpc_access_connector` (subnet / range của connector) hoặc Direct VPC egress (subnet chỉ định); không có VPC egress thì chỉ có public path.
  - CF Workers: chỉ public path, hoặc qua `cf_tunnel`.
- **Đích** (entry point của B, ưu tiên): internal LB → VPC endpoint service → DB endpoint → workload. Gồm `{CIDR / IP set, SG / firewall target, port}`.
- **Dịch vụ không nằm trong VPC** (DynamoDB, S3, GCS): đánh giá route tới gateway endpoint / prefix list (`pl-…`) hoặc Private Google Access; không có thì đi qua NAT (public path).
- **R2 / dịch vụ Cloudflare:** luôn là public path tới endpoint Cloudflare (`<account>.r2.cloudflarestorage.com` hoặc custom domain); nguồn cần egress qua NAT / IGW. CF Worker có binding R2 thì đi `provider-internal` (không qua network của khách hàng).
- **Đích sau Cloudflare Tunnel:** `cf_dns_record` → `resolves_to` tunnel → `cf_tunnel_route` khớp hostname → service nội bộ; tunnel `runs_in` subnet / workload. Đường này được tính như private path từ phía `cloudflared` tới service (đánh giá SG / firewall từ subnet của `cloudflared`), kèm caveat `cf-access-not-evaluated`. Thiếu bất kỳ mắt xích nào thì hop `unknown` với lý do `cf-tunnel-incomplete`.
- Không resolve được thì Path `unknown` với lý do `endpoint-not-linked`.

### 8.3 Tìm đường

BFS trên graph Network (node) và Connectivity + observed (`vpc_peering`, `tgw_attachment`, `vpce`, `vpn_connection`, `interconnect_attachment`, `cf_tunnel`) làm edge. Trả về tối đa **một private path tốt nhất** (ít hop nhất) và **một public path** (nếu B có ingress `public`). PrivateLink / PSC không phải hop route cross-VPC: endpoint nằm trong VPC nguồn.

### 8.4 Đánh giá

**Routing (cả forward và return):**
- Longest-prefix match trên route table của subnet nguồn tới IP / CIDR đích, và của subnet đích về CIDR nguồn (return path bắt buộc với peering / TGW / VPN).
- Route `local`, blackhole route, route propagation từ VGW được tính.
- TGW: route table của attachment (association) và route có được (propagation / static) tới attachment đích; blackhole thì `fail`.
- CIDR chồng lấn giữa hai VPC thì `unknown` (`overlapping-cidr`).

**Security group (AWS):**
- Egress phía nguồn, ingress phía đích; match protocol / port range / CIDR / prefix list / SG-reference.
- SG-reference chỉ hợp lệ trong cùng VPC, qua peering cùng region, hoặc qua TGW khi bật SG referencing; ngoài các trường hợp đó, rule SG-reference **không match** (ghi caveat).
- SG là stateful: không đánh giá return ở tầng SG.

**PrivateLink / PSC:**
- Endpoint ở trạng thái `available` / `accepted`; consumer principal nằm trong allowed principals của endpoint service.
- SG của interface endpoint phải cho phép ingress từ nguồn.
- Phía producer: SG đích phải cho phép từ IP của NLB, hoặc IP client nếu bật client IP preservation.

**GCP firewall:**
- Thứ tự: hierarchical firewall policy → network firewall policy → VPC firewall rule.
- Trong cùng priority, deny thắng allow.
- Implied rules: deny all ingress, allow all egress.
- Match theo target tags / target service accounts / source ranges.
- Source tags không áp dụng qua peering.
- Shared VPC: rule nằm ở host project.
- Firewall là stateful.

**Public path:** nguồn có egress qua NAT / IGW (hoặc Cloud NAT); đích có ingress public (public LB / Cloudflare hostname); không đánh giá WAF / Access.

### 8.5 Gộp verdict

| Điều kiện | Verdict |
|---|---|
| Có private path toàn `pass` | `reachable-with-caveats` nếu có caveat, ngược lại `reachable` |
| Không có private path `pass`, có public path `pass` | `reachable-with-caveats` + caveat `public-only` (kèm finding `public-only-path`) |
| Mọi path có ít nhất một hop `fail` | `blocked` (báo hop `fail` đầu tiên) |
| Còn lại (có `unknown`, không có path `pass`) | `unknown` |

Caveat luôn có ở v1: `nacl-not-evaluated`, `iam-policy-not-evaluated`; thêm `cf-access-not-evaluated`, `k8s-networkpolicy-not-evaluated` khi liên quan. Engine không bao giờ trả `reachable` khi thiếu dữ liệu ở bất kỳ hop bắt buộc nào.

`input_hash` = sha256(`observed_generation` + `landscape_catalog_revision`), tính được trước khi đánh giá nên tra cache trực tiếp được.
- `landscape_catalog_revision` là `max(updatedAt)` trên các kind ảnh hưởng tới kết quả: `Deployment`, `Network`, `Connectivity`, `CloudAccount`, `Service` (vì `dependsOn`), `Infrastructure` (vì `engine`), `DataFlow`. (`Highlight` không ảnh hưởng reachability.)
- Ở quy mô 100–500 deployment, việc vô hiệu hóa toàn cục mỗi khi có thay đổi là chấp nhận được, vì precompute chạy nền.

### 8.6 API và tính trước

- `POST /api/v1/landscape/path` (body Query, trả về Result; dùng cache nếu `input_hash` khớp).
- MCP tool `trace_network_path`.
- Tính lại nền (worker pool, timeout / query) cho các cạnh `connectsTo` (khai báo + suy luận) bị ảnh hưởng khi:
  - có import mới (source chứa endpoint hoặc hop);
  - có thay đổi entity `Deployment` / `Network` / `Connectivity` / `Service.dependsOn` / `DataFlow` (event bus `entity.updated`).
- DataFlow có actor: tính hai chặng actor → source và actor → target như hai Query thường, lưu trong `reachability_results`. Verdict của DataFlow = verdict kém hơn của hai chặng. Nếu `transport` khai báo là private / privatelink / vpc-endpoint mà chặng nào chỉ thông qua public, sinh finding `dataflow-transport-mismatch`.
## 9. Findings và Drift

Framework findings (bảng, engine chạy rule, API, Findings bar) thuộc **Plan A**; mỗi plan sau chỉ đăng ký thêm rule.

Bảng `landscape_findings(id PK, rule, category: finding|drift, severity: error|warning|info, subject_ref, details JSON, first_seen, last_seen, status: open|resolved)`; tự `resolved` khi điều kiện không còn. (Không có ack workflow ở v1.)

| Rule | Plan | Category | Severity | Mô tả |
|---|---|---|---|---|
| `missing-owner` | A | finding | warning | Service / Deployment không có owner |
| `dangling-ref` | A | finding | warning | ref không resolve được |
| `cross-env-dependency` | A | finding | error | `connectsTo` khai báo prod → dev/qc |
| `dataflow-sensitive-to-lower-env` | A | finding | error | DataFlow có `dataClasses` chứa `pii` / `payment` đi từ prod sang dev / qc / sandbox |
| `dataflow-sensitive-public` | A | finding | warning | DataFlow có `pii` / `payment` khai báo `transport: public-internet` |
| `dataflow-no-actor` | A | finding | info | DataFlow `mode` khác `replication` mà không có `actor` |
| `manual-shadowed` | B | drift | info | resource nhập tay trùng `external_id` với resource từ Terraform |
| `declared-not-observed` | B | drift | warning | entity có link nhưng resource đã bị xóa |
| `observed-unmanaged` | B | drift | info | workload / DB observed không link tới Deployment nào |
| `runtime-mismatch` | B | drift | warning | `runtime.platform` khác type observed |
| `connectivity-missing` | B | drift | warning | Connectivity khai báo không có resource tương ứng |
| `dependency-blocked` | C | finding | error | `connectsTo` có verdict `blocked` |
| `dependency-unknown` | C | finding | warning | `connectsTo` có verdict `unknown` |
| `public-only-path` | C | finding | warning | hai deployment nội bộ chỉ thông qua public |
| `cross-env-path` | C | finding | warning | tồn tại đường network từ prod tới dev/qc (không có cạnh khai báo) |
| `dataflow-blocked` | C | finding | error | một chặng của DataFlow (actor → source / target) `blocked` |
| `dataflow-unknown` | C | finding | warning | một chặng của DataFlow `unknown` |
| `dataflow-transport-mismatch` | C | finding | warning | `transport` khai báo private nhưng thực tế chỉ thông qua public |

## 10. API cho Landscape

Tất cả yêu cầu `landscape:read`, đồng thời lọc entity qua `FilterEntitiesByAccess` hiện có.

- `GET /api/v1/landscape/graph?groupBy=org|cloud|env&level=company|project|deployment&layers=network,data&env=&cloud=&region=&runtime=&team=&project=&department=&lifecycle=` (Plan A) trả về `{clusters, nodes, edges (aggregate ở level cao), counts, highlights}`.
  - `edges[].layer` = `network` (`connectsTo`) hoặc `data` (DataFlow). Cạnh data đi từ `source` tới `target`, mang `{dataFlowRef, actorRef, mode, transport, dataClasses}`; ở level cao được gộp và đếm như cạnh network. Actor được trả như node bình thường, thêm `actorOf[]`.
  - `highlights[]` = mọi Highlight có ít nhất một match trong phạm vi hiện tại: `{ref, name, color, defaultOn, nodeIds[], edgeIds[]}`. Match được tính phía server (selector cần dữ liệu catalog đầy đủ) sau khi đã lọc quyền. UI bật / tắt phía client, không cần gọi lại.
- `GET /api/v1/landscape/dataflows?...` (Plan A) trả về danh sách DataFlow dạng bảng cùng bộ filter, kèm verdict hai chặng khi có Plan C.
- `GET /api/v1/landscape/inventory?...` (Plan A) trả về ma trận Service × Env.
- `GET /api/v1/landscape/blast-radius/{kind}/{ns}/{name}` (Plan A, chỉ dựa trên catalog: duyệt ngược `connectsTo` + `dependsOn`, cộng các DataFlow có subject là `source` / `target` / `actor`; gộp theo Project / Department).
- `GET /api/v1/landscape/network?...` (Plan C) trả về cây Cloud › Account › Region › Network › Subnet + deployment, ingress, connectivity.
- `POST /api/v1/landscape/path` (Plan C).
- `GET /api/v1/landscape/findings?category=&severity=&rule=&status=` (Plan A; rule của B và C xuất hiện khi các plan đó xong).

## 11. UI

Trang mới `web/src/pages/Landscape.tsx`, thay thế `TopologyExplorer` (route cũ redirect). Mockup tham khảo: Open Design `gantry-landscape`.

- **Top bar:** breadcrumb phạm vi; filter chips Env / Cloud / Region / Runtime / Owner team / Lifecycle; search; nhãn "Dữ liệu observed cập nhật lúc …" (không có snapshot selector ở v1). Toàn bộ state nằm trong URL query (chia sẻ bằng link; không có Saved views ở v1).
- **Trái:** cây Department › Project › Service kèm số đếm; click để zoom phạm vi.
- **Giữa:** 4 lens.
  1. **Map (A):** semantic zoom Company › Project › Deployment; Group by Org | Cloud | Env; cluster collapse / expand; edge aggregate có độ dày theo số lượng; dải "Shared platform".
     - **Lớp cạnh:** toggle `Network` / `Data flows` / cả hai (URL `layers=`). Cạnh data vẽ khác hẳn cạnh network: nét gạch có mũi tên chuyển động nhẹ, icon theo `mode` (batch / stream / replication…), nhãn `schedule` khi hover; actor hiện như chip nhỏ giữa cạnh, click để chọn actor. Cạnh có `pii` / `payment` có viền `--gantry-warning`.
     - **Panel "Layers"** (nổi ở góc map): danh sách Highlight trong phạm vi, mỗi dòng có chấm màu, tên, số match, checkbox bật / tắt (URL `hl=`). Khi có ít nhất một highlight bật: phần match giữ nguyên độ đậm và có viền / glow màu của highlight, phần còn lại mờ xuống; node thuộc nhiều highlight hiện nhiều vòng màu. Nút "Chỉ hiện phần highlight" ẩn hẳn phần không match.
     - **Đánh dấu nhanh:** chọn nhiều node / cạnh (shift-click hoặc kéo vùng) → "Thêm vào highlight…" (chọn có sẵn hoặc tạo mới với tên + màu). Ghi qua API entity như mọi kind khác (cần `entity:write`).
  2. **Inventory (A):** ma trận Service × Env group theo Project; ô có runtime / region / version / cloud + badge; KPI strip. Tab phụ **Data flows**: bảng DataFlow (source, target, actor, mode, schedule, transport, dataClasses, owner, verdict khi có Plan C), click một dòng thì chuyển sang Map với luồng đó được highlight tạm.
  3. **Network (C):** container lồng Cloud › Account › Region › VPC › Subnet; deployment là pill; Connectivity theo style (peering liền, PrivateLink / PSC đứt, VPN / Interconnect đôi, CF Tunnel chấm, public chấm đỏ); ingress ở mép VPC.
  4. **Path (C):** form From / To / protocol / port; banner verdict + caveats; dải hop (forward + return) có evidence mở rộng; public path thay thế nếu có.
- **Inspector (phải):** chi tiết node, kết nối ra / vào + verdict (khi có Plan C), DataFlow liên quan (vai trò source / target / actor), highlight chứa node, findings, observed resources đã link (badge "Manual" nếu nhập tay), nút "Trace path tới…" và "Blast radius". Chọn một cạnh data thì Inspector hiện chi tiết DataFlow và hai chặng actor → source / target (Plan C, mỗi chặng mở được sang Path lens).
- **Findings bar (dưới):** tab Findings (A), Drift (B), Link suggestions với Confirm / Dismiss (B).
- **Thư viện:** `@xyflow/react` + `elkjs`, lazy-load theo route (`React.lazy`) vì `elkjs` khoảng 1.5 MB; layout ELK chạy trong Web Worker.
- **Theming:** chỉ dùng `--gantry-*` có trong `web/src/styles/globals.css`, gồm cả `--gantry-success` / `--gantry-warning` (đã tồn tại). Thêm 6 biến `--gantry-mark-1..6` (bảng màu phân biệt được cho highlight, có giá trị riêng cho light / dark, đủ tương phản trên `--gantry-bg-primary`). Cập nhật `AGENTS.md` / `CLAUDE.md` cho khớp (hiện ghi chỉ có 9 biến).

## 12. MCP tools mới

| Tool | Plan | Quyền |
|---|---|---|
| `migrate_landscape` | A | developer (`entity:write`) |
| `get_landscape_inventory`, `get_blast_radius` | A | `landscape:read` |
| `mark_entities {highlight, add[], remove[], addEdges[], removeEdges[]}` (tạo Highlight nếu chưa có, khi kèm `color`) | A | `entity:write` |
| `get_landscape_dataflows` | A | `landscape:read` |
| `import_terraform_state`, `list_observed_sources` | B | `observed:write` / `observed:read` |
| `upsert_manual_resources`, `delete_manual_resource` | B | `observed:write` |
| `list_observed_resources` | B | `observed:read` |
| `list_link_suggestions`, `confirm_link`, `dismiss_link` | B | `observed:read` / `observed:write` |
| `list_findings` | A | `landscape:read` |
| `trace_network_path` | C | `landscape:read` |

`link_service` hiện có thêm `targetNamespace`. Kind mới (kể cả `DataFlow`, `Highlight`) dùng `create_entity` / `update_entity` sẵn có (schema tự xuất hiện qua `get_schema`).

Ví dụ khai báo job copy R2 sang S3 qua MCP: `create_entity` Infrastructure `r2-game-assets` (`engine: r2`, `managedBy: cloudflare`) → `create_entity` DataFlow `{source: Infrastructure/music/r2-game-assets, target: Deployment/music/s3-app-storage-production, actor: Deployment/music/asset-sync-production, mode: batch, schedule: "0 */6 * * *", transport: public-internet, dataClasses: [assets]}` → (tùy chọn) `mark_entities {highlight: cross-cloud-egress, color: mark-3, add: [DataFlow/music/r2-to-s3-assets]}`. Nếu tunnel / DNS liên quan không có trong Terraform thì nhập bằng `upsert_manual_resources`.

## 13. Xử lý lỗi

- Import: xem 7.2 bước 7.
- Normalizer gặp type lạ: lưu `generic`, tăng metric `gantry_observed_unknown_types_total{native_type}`.
- Reachability timeout / thiếu dữ liệu: verdict `unknown` với lý do cụ thể.
- Ref không resolve được: chấp nhận ghi, sinh `dangling-ref`; cạnh suy luận liên quan bị bỏ qua.

## 14. Kiểm thử

- **Plan A:** unit cho `ResolveRef` / `ParseCanonicalRef` và từng call site đã đổi; schema validation kinds mới; suy luận `connectsTo`; migration dry-run trên fixture bằng catalog hiện tại cho ra đúng 22 Deployment; API graph / inventory / blast-radius; DataFlow trong graph (layer `data`, gộp ở level cao, blast radius qua actor); Highlight: match theo members / edges / selector, kết hợp AND / OR, lọc quyền không lộ member bị ẩn; `mark_entities`; rule `dataflow-*` của Plan A; `npx tsc --noEmit` và `npm run build`.
- **Plan B:** sanitizer với fixture chứa secret (outputs, sensitive_attributes, env vars, tag value) và assert không lọt vào DB, log hay response; normalizer từng provider với fixture tfstate đã ẩn danh từ `terraform-ama-nakama-upe-*`; idempotency theo serial; quyền ghi đè source; auto-link; nhập tay: từ chối attr ngoài allowlist / denylist, `toKey` không tồn tại, type không cho phép; upsert cùng `key` là cập nhật; nhiều principal cùng sửa một source manual; `manual-shadowed`; tag allowlist với `extra` pattern và `replaceDefaults`, `gantry.io/*` luôn được giữ.
- **Plan C:** golden topology cho reachability: same-VPC, peering cùng / khác region (SG-ref), TGW có / không propagation, blackhole, thiếu return route, overlapping CIDR, PrivateLink (allowed principals, NLB SG), GCP firewall (deny cùng priority, hierarchical policy, Shared VPC), Cloud Run connector, DynamoDB gateway endpoint, public-only, đích sau CF Tunnel (đủ / thiếu mắt xích, dữ liệu manual), R2 qua NAT, DataFlow hai chặng và `dataflow-transport-mismatch`; findings rules; seed khoảng 500 deployment để kiểm tra hiệu năng UI và precompute.

## 15. Chia implementation plan

| Plan | Nội dung | Phụ thuộc |
|---|---|---|
| **A: Model + Map / Inventory** | Ref canonical + namespace, kinds mới / sửa (gồm `DataFlow`, `Highlight`), `connectsTo` suy luận, migration, RBAC `landscape`, API graph (layer network + data, highlights) / inventory / dataflows / blast-radius, framework findings + rule catalog-only, UI Landscape (Map với lớp Data flows và panel Layers, Inventory + tab Data flows, Inspector cơ bản, Findings bar), biến `--gantry-mark-*`, MCP plan A | không |
| **B: Observed + importer** | Bảng observed + `observed_generation`, RBAC `observed` + service user, sanitizer, tag allowlist cấu hình được, normalizer AWS rồi GCP rồi Cloudflare, REST / MCP import, nhập tay resource qua MCP (source `manual`), auto-link + suggestions UI, drift rules | A |
| **C: Reachability + Network / Path** | Engine reachability (gồm CF Tunnel, R2, hai chặng DataFlow), precompute + cache, rule reachability + `dataflow-*`, Network lens, Path lens | A, B |

### Cố ý loại khỏi v1 (YAGNI)

Saved views (Highlight thay thế phần "đánh dấu để chia sẻ"; filter vẫn chia sẻ qua URL), snapshot time-travel, Department lồng nhau, k-shortest paths, ack workflow cho findings, phân quyền theo CloudAccount / namespace, đánh giá NACL / IAM / Cloudflare Access / k8s NetworkPolicy, Cloudflare API, UI form để nhập tay resource observed (v1 chỉ qua MCP), DataFlow nhiều bước (pipeline A → B → C mô hình bằng nhiều DataFlow nối tiếp), lineage mức dataset / bảng.

## 16. Quyết định đã chốt từ câu hỏi mở (v3)

- **Cloudflare:** không cần Cloudflare API ở v1. Phần Terraform không quản lý (tunnel, DNS, R2, Workers) nhập tay qua MCP (6.3).
- **Tag allowlist:** cấu hình tự do qua `observed.tags`, có thể thêm key nội bộ hoặc không (6.4).
- **Luồng dữ liệu / job:** mô hình bằng kind `DataFlow` (source, target, actor); đánh dấu / highlight bằng kind `Highlight`; cả hai thuộc Plan A, phần reachability của DataFlow thuộc Plan C.

Hiện không còn câu hỏi mở.
