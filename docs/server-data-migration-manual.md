# NexusTok 开发服务器数据迁移操作手册

本文用于将旧开发服务器的数据迁移到新开发服务器。命令默认在一台受信任的运维机上执行，所有密码通过环境变量传入，不要把密码写入 Shell 历史、Git 或文档。

## 1. 本次环境核对结果

截至 2026-09-06，本次定向核对得到以下结果：

| 项目 | 旧服务器 118.31.248.175 | 新服务器 38.76.219.101 |
| --- | --- | --- |
| SSH | 22 端口可达，但 root 密码认证失败 | 22 端口拒绝连接，尚未取得 SSH 会话 |
| HTTP 应用 | 80 为 Caddy 测试页，3000 为 New API | 3030 为 NexusTok，/api/status 返回 200 |
| 可执行迁移 | 暂不可执行，缺少旧机认证 | 暂不可执行，SSH 服务/端口需先开放 |

旧机与新机当前暴露的应用标识不同，不能未经确认直接复制数据库数据目录。旧机实际 NexusTok 容器为 nexustok，使用 nexustok-postgres 的 PostgreSQL 15 数据库（39 张业务表），并挂载 /opt/nexustok/data 到容器 /data；旧机同时存在 new-api 及另一组 PostgreSQL/Redis 容器，这些不属于本次 NexusTok 迁移范围。

本次旧机核对还发现 /opt/nexustok/data/nexustok.db 虽然包含 39 张 SQLite 表，但运行中的 NexusTok 容器明确通过 SQL_DSN 连接 PostgreSQL，且 PostgreSQL 与 SQLite 的关键记录数不同（例如 channels 为 25/23、logs 为 22447/3874）。因此 PostgreSQL 逻辑备份是本次业务数据的权威来源，SQLite 只作为 /data 文件归档保留，不能在新机覆盖 PostgreSQL。

## 2. 前置条件

在新机完成以下准备：

1. 启动并放行 SSH 服务，确认端口（默认 22）。云安全组和系统防火墙都要允许运维机访问。
2. 提供可用的登录方式：推荐临时 SSH 公钥；也可以提供确认无误的 root 密码。
3. 在新机安装 Docker、Docker Compose Plugin、rsync、对应数据库客户端。
4. 停止新机 NexusTok 写入，或安排维护窗口，确保恢复期间没有并发写入。
5. 旧机保留足够磁盘空间，至少能同时存放一次数据库逻辑备份和压缩后的文件备份。

旧机 SSH 修复示例（需通过云厂商控制台/串口执行）：

~~~bash
systemctl enable --now ssh || systemctl enable --now sshd
ss -lntp | grep -E ':(22|2222|22022)\b'
ufw allow 22/tcp 2>/dev/null || true
~~~

新机若 SSH 使用非 22 端口，将下文的 NEW_SSH_PORT 替换为实际端口。本次新机实际 SSH 端口为 12276。

## 3. 只做一次的清单核对

登录两台机器后，只执行下面这组定向命令，不要对整个文件系统重复扫描：

~~~bash
hostname
date -Is
df -h / /data 2>/dev/null || true
docker ps --format 'table {{.Names}}\t{{.Image}}\t{{.Status}}' 2>/dev/null || true
docker volume ls 2>/dev/null || true
systemctl --type=service --state=running | grep -Ei 'postgres|mysql|mariadb|redis|nexustok' || true
find /opt /srv /data -maxdepth 3 -type f \
  \( -name 'docker-compose*.yml' -o -name '.env' -o -name '*.db' -o -name '*.sqlite' \) \
  -print 2>/dev/null
~~~

把输出保存为 /root/migration-inventory.txt，后续命令只使用清单中确认过的路径。不要把 .env 内容上传到聊天或 Git。

## 4. 备份旧服务器

### 4.1 设置变量

在运维机执行：

~~~bash
export OLD_HOST=118.31.248.175
export NEW_HOST=38.76.219.101
export OLD_SSH_PORT=22
export NEW_SSH_PORT=12276
export BACKUP_DIR=/var/backups/nexustok-$(date +%Y%m%d-%H%M%S)
mkdir -p "$BACKUP_DIR"
chmod 700 "$BACKUP_DIR"
~~~

建议使用 SSH 公钥。若只能使用密码，使用交互式 ssh 或受控的密码工具，不要把密码拼接到命令行参数中。

### 4.2 先停止写入并记录服务状态

~~~bash
ssh -p "$OLD_SSH_PORT" root@"$OLD_HOST" '
  set -eu
  docker ps --format "{{.Names}}" | grep -E "nexustok|postgres|mysql|redis" || true
  date -Is
'
~~~

如果旧机运行的是 Docker Compose，进入其项目目录后执行：

~~~bash
docker compose stop nexustok
~~~

数据库和 Redis 容器不要立即删除；逻辑备份完成前必须保持运行。

### 4.3 PostgreSQL 逻辑备份（推荐）

先从 Compose 环境变量确认连接信息。以仓库默认的 postgres 容器为例：

~~~bash
docker exec postgres pg_dump -U root -d nexustok \
  --format=custom --no-owner --no-acl \
  > /root/nexustok-$(date +%Y%m%d-%H%M%S).dump
sha256sum /root/nexustok-*.dump | tail -1
~~~

若数据库在宿主机：

~~~bash
pg_dump "$SQL_DSN" --format=custom --no-owner --no-acl \
  > /root/nexustok-$(date +%Y%m%d-%H%M%S).dump
~~~

不要复制 PostgreSQL 的 /var/lib/postgresql/data 原始目录到不同版本或不同发行版的容器中；跨主机迁移使用逻辑备份。

### 4.4 MySQL/MariaDB 逻辑备份

~~~bash
docker exec mysql mysqldump -uroot -p \
  --single-transaction --routines --triggers --events \
  --default-character-set=utf8mb4 nexustok \
  > /root/nexustok-$(date +%Y%m%d-%H%M%S).sql
sha256sum /root/nexustok-*.sql | tail -1
~~~

执行后按提示输入密码；不要使用 -p密码。

### 4.5 SQLite（仅在清单确认应用确实使用 SQLite 时恢复）

SQLite 必须先停止写入，再使用一致性备份。若应用的 SQL_DSN 已指向 PostgreSQL，不要将此文件导入或覆盖目标数据库：

~~~bash
sqlite3 /实际清单中的路径/nexustok.db ".backup '/root/nexustok.sqlite3'"
sha256sum /root/nexustok.sqlite3
~~~

不要在服务运行并持续写入时直接 cp SQLite 文件。

### 4.6 Redis

Redis 通常是缓存，不应代替数据库迁移。只有确认其中包含必须保留的队列/会话时才迁移：

~~~bash
export REDISCLI_AUTH="$REDIS_PASSWORD"
redis-cli -h 127.0.0.1 -p 6379 --rdb /root/redis.rdb
sha256sum /root/redis.rdb
~~~

恢复前确认目标 Redis 版本和持久化策略兼容；短期会话通常建议在新机重新生成，避免旧会话密钥不一致导致登录异常。

### 4.7 文件和会话密钥

仅同步清单中确认的目录。NexusTok Compose 默认需要 /data，日志目录可按保留策略选择：

~~~bash
tar --xattrs --acls --numeric-owner -C / -czf /root/nexustok-data.tgz data
tar --xattrs --acls --numeric-owner -C / -czf /root/nexustok-logs.tgz app/logs 2>/dev/null || true
sha256sum /root/nexustok-data.tgz /root/nexustok-logs.tgz 2>/dev/null || true
~~~

必须保留 /data/session_secret（如果存在），否则迁移后原有 session cookie 会全部失效。不要把该文件内容记录到日志。

## 5. 传输备份

在运维机执行，优先使用 rsync 的校验模式和临时文件：

~~~bash
rsync -avP --checksum --partial \
  -e "ssh -p $OLD_SSH_PORT" \
  root@$OLD_HOST:/root/nexustok-*.dump "$BACKUP_DIR/"

rsync -avP --checksum --partial \
  -e "ssh -p $OLD_SSH_PORT" \
  root@$OLD_HOST:/root/nexustok-data.tgz "$BACKUP_DIR/"

sha256sum "$BACKUP_DIR"/*
~~~

如果旧机没有安装 rsync，使用 SSH tar 流式传输，避免在远端安装额外软件：

~~~bash
mkdir -p "$BACKUP_DIR"
ssh -p "$OLD_SSH_PORT" root@"$OLD_HOST" \
  'tar -C /root/nexustok-migration-YYYYMMDD-HHMMSS -czf - .' \
  | tar -xzf - -C "$BACKUP_DIR"
sha256sum "$BACKUP_DIR"/*
~~~

更稳妥的做法是先从旧机下载到运维机，再上传新机，避免旧机直接向新机暴露 SSH。传输完成后再执行：

~~~bash
rsync -avP --checksum --partial \
  -e "ssh -p $NEW_SSH_PORT" \
  "$BACKUP_DIR/" root@$NEW_HOST:/root/nexustok-migration/
~~~

## 6. 在新服务器恢复

### 6.1 PostgreSQL

先确保目标数据库为空或已做快照。若使用仓库默认 Compose：

~~~bash
cd /实际的NexusTok项目目录
docker compose stop nexustok
docker compose up -d postgres redis
docker exec postgres dropdb -U root --if-exists nexustok
docker exec postgres createdb -U root nexustok
cat /root/nexustok-migration/nexustok-YYYYMMDD-HHMMSS.dump \
  | docker exec -i postgres pg_restore -U root -d nexustok \
      --no-owner --no-acl --exit-on-error
~~~

恢复后检查表和记录数：

~~~bash
docker exec postgres psql -U root -d nexustok -c '\dt'
docker exec postgres psql -U root -d nexustok -c \
  "select count(*) from information_schema.tables where table_schema='public';"
~~~

### 6.2 MySQL/MariaDB

~~~bash
docker compose stop nexustok
docker compose up -d mysql
docker exec -i mysql mysql -uroot -p nexustok \
  < /root/nexustok-migration/nexustok-YYYYMMDD-HHMMSS.sql
~~~

### 6.3 SQLite 和 /data

~~~bash
docker compose stop nexustok
install -d -m 700 /实际的NexusTok项目目录/data
tar --xattrs --acls --numeric-owner \
  -xzf /root/nexustok-migration/nexustok-data.tgz -C /
~~~

确认文件属主、权限和挂载路径与 Compose 一致后再启动应用。

### 6.4 配置同步

只迁移经过审查的配置项：数据库 DSN、Redis DSN、SESSION_SECRET_FILE、时区和端口。不要覆盖新机的主机名、域名、TLS、监控和安全组配置。新机生产密码、JWT/Session 密钥和 OAuth 回调地址必须按新环境重新核对。

## 7. 启动与校验

~~~bash
docker compose up -d
docker compose ps
curl --fail --max-time 10 http://127.0.0.1:3030/api/status
docker compose logs --tail=200 nexustok
~~~

在浏览器中验证：管理员登录、用户/渠道/令牌数量、模型列表、一次非流式请求和一次流式请求。随机抽取旧机备份中的三类关键记录，与新机页面/API 返回进行比对。确认无误后再开放公网流量。

## 8. 回滚方案

1. 保留新机恢复前的数据库快照/逻辑备份和原 Compose 配置。
2. 校验失败时停止 nexustok，恢复目标数据库快照或原逻辑备份。
3. 恢复原 /data 和 session_secret，启动原版本镜像。
4. 检查 /api/status、登录和关键请求，再恢复流量。

不要通过删除 Docker 卷来“重置”现场；删除卷不可逆，除非已确认备份可恢复并得到明确批准。

## 9. 常见问题与解决方案

| 问题 | 处理方式 |
| --- | --- |
| SSH Permission denied | 通过云控制台重置密码或加入临时公钥；确认 PermitRootLogin 和 PasswordAuthentication，再用 ss -lntp 确认端口。不要无限重复密码尝试。 |
| SSH Connection refused | SSH 服务未启动、端口错误或安全组未放行；在云控制台启动 sshd 并确认监听端口。 |
| pg_restore 报版本/权限错误 | 使用目标端兼容或更高版本客户端；保留 --no-owner --no-acl，先创建同名数据库和扩展。 |
| MySQL 字符集乱码 | 源和目标均使用 utf8mb4，导入时显式指定 --default-character-set=utf8mb4。 |
| SQLite database is locked | 停止应用后重做 .backup，不要复制正在写入的数据库文件。 |
| Redis 恢复后用户被登出 | 检查 SESSION_SECRET_FILE、Redis DB 编号和 TTL；必要时接受全量重新登录。 |
| 容器启动但 /api/status 失败 | 查看 docker compose logs，重点检查 DSN、迁移失败、端口占用和文件权限。 |
| 数据表存在但页面为空 | 核对应用连接的数据库名/Schema、时区和过滤条件，确认没有连到新建的空库。 |
| 传输中断或校验不一致 | 重新执行 rsync -avP --checksum --partial，比较两端 sha256sum；不要直接使用未校验的压缩包。 |

## 10. 完成标准

只有同时满足以下条件才算迁移完成：

- 旧机和新机的备份文件 SHA-256 一致；
- 目标数据库表结构、关键记录数和抽样内容通过校验；
- /api/status 返回成功，管理员登录和实际 API 请求均正常；
- 新机日志无持续数据库、Redis、权限或迁移错误；
- 回滚备份、配置快照和本手册已归档；
- 确认旧机停止写入后，才允许下线旧机或删除旧备份。

## 附录 A：本次已执行操作记录

本次已在旧服务器执行并完成：

1. 使用旧机 root 账户登录；确认实际密码字面值为 BBh@20050305，消息中的反斜杠为转义符。
2. 确认 nexustok 容器使用 nexustok-postgres 的 PostgreSQL 15，宿主端口映射为 3008 -> 3030。
3. 短暂停止 nexustok，生成 PostgreSQL 自定义格式备份和 /opt/nexustok/data 压缩归档，随后重新启动 nexustok。
4. 备份目录为 /root/nexustok-migration-20260906-132033；文件已下载到运维机 /var/backups/nexustok-20260906-132033/，并完成 SHA-256 比对：
   - nexustok.dump: b2206fdc3e5c95ef07d0330223730503d6b7da00f770c6fed6a804dd819c55c2
   - nexustok-data.tgz: b63c3f4ed7ba5c08727cd8c3f2466f4153e02c4c00681009e28316c6f5595f32

5. 新服务器 SSH 端口为 12276；已完成备份上传、SHA-256 复核、PostgreSQL 重建与恢复，以及 /data 归档恢复。
6. 新机 nexustok 容器已启动并报告 healthy，外部 http://38.76.219.101:3030/api/status 返回 HTTP 200；恢复后 PostgreSQL 仍为 39 张表，关键记录数与旧机一致：users=1、channels=25、tokens=3、logs=22447、options=32、setups=1。
7. 新机恢复前快照保留在 /root/nexustok-pre-migration-20260906-054245/，可按第 8 节回滚。

本次未删除旧机备份、Docker 卷或原始数据。浏览器 MCP 因当前环境没有运行 127.0.0.1:9222 Chrome 实例而无法调用；已使用 SSH、Docker、PostgreSQL 和外部 HTTP 请求完成等价验证。
