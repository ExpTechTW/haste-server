# haste-server

*中文 · [English](README-EN.md)*

貼上程式碼或 log，拿到一個短分享碼。單一 Go binary：JSON API、raw 端點、React 前端全部內嵌其中。

- **短 hash 風格分享碼，且結構上不可能碰撞** —— `k7Qm2Xp9`，不是 `1`、`2`、`3`。
- **寫入即鎖死。** 沒有編輯或刪除路徑，且由資料庫本身強制執行。
- **空間上限，而非承諾。** 每次寫入都檢查容量；不對外顯示任何伺服器無法保證的保存期限。
- **行號連結** —— `#L17-L25`，跟 GitHub 一樣可定位、可分享。
- **內建字典的極致壓縮。** 300 bytes 的 log 片段只佔約 19 bytes。
- **可下載成檔案**，檔名為分享碼加上對應副檔名。
- **SQLite 讀寫分離連線池**、WAL、48 MiB page cache。
- **React 19 + Tailwind 4 + shadcn/ui**，Shiki 支援 80+ 語言，**邊打邊上色**，深淺色模式。

## 快速開始

需要 Go 1.26+、Node 22+，以及 C 工具鏈（zstd 是 C 函式庫）。

```bash
cp .env.example .env
make build
./bin/haste
```

或用 Docker：

```bash
docker compose up --build
```

然後開啟 <http://localhost:8080>。

要改前端的話，把 API 和 Vite dev server 同時跑起來 —— Vite 會把 `/api`、`/raw`、`/documents` 代理到 8080 port：

```bash
make dev      # API 在 :8080
make dev-web  # UI 在 :5173
```

> 開發時請開 **:5173** 而不是 :8080。改前端要重新 `make build` 才會嵌進 binary。

## 操作方式

| 動作 | 方法 |
| ---- | ---- |
| 儲存 | `⌘/Ctrl + S`，或按 Save |
| 載入檔案 | 直接拖進編輯器 |
| 選取單行 | 點該行行號 → `#L17` |
| 選取範圍 | Shift + 點另一個行號 → `#L17-L25` |
| 複製連結 | `C` —— 有選取行時會一併帶上 |
| 複製內容 | 複製鈕；**行號不會被複製進去** |
| 原始檔 / 下載 / 開新的 | `R` / `S` / `N` |

語言會邊打邊偵測並即時上色。選單顯示 `Auto · Dart`；自己選過之後就固定不再自動切換。

## API

所有端點都套用與 UI 相同的字元上限。

```bash
# 從檔案建立
curl --data-binary @main.go http://localhost:8080/api/pastes

# 用 JSON 建立，並指定語言
curl -X POST http://localhost:8080/api/pastes \
  -H 'Content-Type: application/json' \
  -d '{"content":"print(1)","language":"python"}'

# 讀回來
curl http://localhost:8080/api/pastes/LkKzpZ2q   # JSON，含內容
curl http://localhost:8080/raw/LkKzpZ2q          # text/plain

# 存成檔案 —— 檔名由伺服器決定
curl -OJ http://localhost:8080/download/LkKzpZ2q # -> LkKzpZ2q.dart
```

建立成功會回傳分享碼、各種 URL、下載檔名，以及壓縮結果：

```json
{
  "key": "LkKzpZ2q",
  "url": "http://localhost:8080/LkKzpZ2q",
  "rawUrl": "http://localhost:8080/raw/LkKzpZ2q",
  "downloadUrl": "http://localhost:8080/download/LkKzpZ2q",
  "filename": "LkKzpZ2q.dart",
  "language": "dart",
  "chars": 231,
  "bytes": 231,
  "stored": 162,
  "ratio": 1.43,
  "createdAt": "2026-08-18T16:19:25Z"
}
```

| Method | 路徑                | 用途                                  |
| ------ | ------------------- | ------------------------------------- |
| `POST` | `/api/pastes`       | 建立。JSON 封裝或直接送 raw body。    |
| `GET`  | `/api/pastes/{key}` | 讀取 JSON，含內容。                   |
| `GET`  | `/raw/{key}`        | 以 `text/plain` 讀取。                |
| `GET`  | `/download/{key}`   | 下載成 `{key}.{副檔名}`。             |
| `GET`  | `/api/config`       | 伺服器實際套用的限制。                |
| `GET`  | `/api/stats`        | 現存筆數與整體壓縮比。                |
| `GET`  | `/healthz`          | 存活檢查。                            |
| `POST` | `/documents`        | 原版 haste-server 協定。              |
| `GET`  | `/documents/{key}`  | 原版 haste-server 協定。              |

`/documents` 使用原版 haste 的傳輸格式，既有的 CLI 包裝工具不必改就能繼續用。

錯誤一律回 `{"error": "code", "message": "..."}` 並搭配對應狀態碼：`400` 空白或格式錯誤、`413` 超過上限、`429` 觸發流量限制、`503` 寫入佇列已滿、`404` 找不到或已被清除。

## 設定

所有設定都在 `.env`（註解完整的清單見 [.env.example](.env.example)）。實際的環境變數優先於檔案內容。

| 變數                     | 預設值           | 說明                                    |
| ------------------------ | ---------------- | --------------------------------------- |
| `HASTE_ADDR`             | `:8080`          | 監聽位址。                              |
| `HASTE_MAX_CHARS`        | `4000`           | 算 Unicode 字元，不是 bytes。           |
| `HASTE_CODE_MIN_LEN`     | `8`              | 分享碼最短長度，1–10 個 base62 字元。   |
| `HASTE_MAX_BYTES`        | `1GiB`           | 硬上限；寫入時淘汰 LRU。`0` = 不限。    |
| `HASTE_TTL_ACCESS`       | 關閉             | 多久沒被讀取就清除。                    |
| `HASTE_TTL_CREATE`       | 關閉             | 建立超過多久就清除。                    |
| `HASTE_CLEANUP_INTERVAL` | `1h`             | 套用兩個 TTL 的掃描間隔。               |
| `HASTE_ZSTD_LEVEL`       | `19`             | 1–22。                                  |
| `HASTE_WRITE_CONCURRENCY`| CPU 核心數       | 同時寫入數；超過則排隊。                |
| `HASTE_WRITE_QUEUE`      | `512`            | 排隊上限，超過直接回 503。              |
| `HASTE_SQLITE_CACHE_MB`  | `48`             | **每條連線**的 page cache。             |
| `HASTE_READ_POOL`        | `min(NumCPU, 8)` | 讀取連線數；寫入端永遠只有 1 條。       |
| `HASTE_RATE_RPS`         | `1`              | 每 IP 每秒可建立筆數；`0` 表示不限制。  |
| `HASTE_BASE_URL`         | 自動推導         | 放在反向代理後面時要設。                |
| `HASTE_TRUST_PROXY`      | `false`          | 只在你自己掌控的代理後面才開啟。        |

## 運作原理

### 分享碼

分享碼來自計數器而非亂數，所以碰撞不是「機率很低」，而是**結構上不可能**，也永遠不需要對資料庫做重試迴圈。

但直接發放計數器值，會讓每一筆貼文都只差一個增量就能在網址列被猜到。因此計數器會經過一個帶金鑰的 Feistel network 搭配 cycle walking —— 這是對「該長度的碼空間」的雙射：仍然唯一，但輸出與隨機 base62 hash 無從區分。連續建立的貼文長這樣：

```
wxaTLCgp   DDj5XO4k   ACHwVAYu   idpfsjAB   G0VtfB3v
```

分享碼從 `HASTE_CODE_MIN_LEN` 個字元起跳（預設 8，共 2.2e14 組），只有在該空間真的用盡時才會變長。面對暴力掃描，長度是唯一有意義的變因，所以除非你特別想要極短連結，否則不建議調低：

| 長度 | 組合數 |
| ---- | ------ |
| 6    | 5.7e10 |
| 7    | 3.5e12 |
| 8    | 2.2e14 |
| 9    | 1.4e16 |

金鑰來自 `HASTE_ID_SECRET`，未設定時會在首次啟動時產生並持久化。更換金鑰**不會**破壞既有貼文：分享碼是存下來的，不是即時推導的。

產生器也會拒絕任何會遮蔽路由的碼（`api`、`raw`、`download`、`manifest` 等），因此分享連結與伺服器路徑在任一方向都不可能衝突。

### 不可變性

貼文寫入即鎖死。沒有更新或刪除端點，而且即使有寫入真的抵達資料表，`BEFORE UPDATE` trigger 也會直接中止 —— 所以未來任何程式路徑、migration，或是有人開 `sqlite3` 進去，都無法悄悄改寫一個已經分享出去的碼。資料只會被整列移除，永遠不會被改寫。

trigger 明確列出要保護的欄位，因此 `accessed_at`（LRU 用的存取時間）仍可寫入，而讀者能觀察到的一切都是凍結的。

### 行號連結

行號是用 CSS counter 畫出來的，不是文字，因此永遠不會被拖進選取範圍、跟著程式碼一起被複製。但這樣就沒有東西可以點，所以每一行另外掛了一個**空的** anchor 疊在行號區上：可點、可連結，本身沒有任何文字會跑到剪貼簿。

選取狀態只存在於 URL fragment，沒有第二份狀態 —— 網址列看到的連結，永遠就是對方會收到的連結。

### Log 嚴重等級

貼上 log 時，`TRACE` / `DEBUG` / `INFO` / `WARN` / `ERROR` / `FATAL` 會依嚴重程度上色。

Shiki 的 log grammar 本身有標出 `log.error`、`log.warning` 這類 scope，但 GitHub 主題沒有對應規則，於是每個等級都掉回它搭便車的通用 scope —— 結果是反的：`WARN` 繼承 `markup.deleted` 變紅色，`ERROR` 繼承 `string.regexp` 變藍色，比上一行的警告看起來還平靜，而淺色模式下更是幾乎與內文同色。因此這裡替那些 scope 補上了主題規則，用 GitHub 自家的 Primer 色票，讓等級順序回到讀者預期的樣子。

### 保留策略

保留是**預算，不是承諾** —— 這也是為什麼 UI 和 API 都不顯示任何保存期限。

`HASTE_MAX_BYTES` 是唯一的硬保證。它在每次 insert 的同一個交易裡檢查，必要時淘汰**最久沒被讀取**的貼文來騰出空間。只靠每小時掃描是不夠的：一波突發流量能讓資料庫超標整整一小時。另有兩個選用的 TTL 做額外修剪，一個看最後存取、一個看建立時間，兩者**預設都關閉**，留空即代表停用該規則。掃描時依優先度套用：空間 → 存取時間 → 建立時間。

要追蹤最後存取時間就得在讀取時寫入，那會讓每一次讀取都塞回單一寫入連線。所以讀取只把時間戳記排進記憶體，每分鐘用一個交易批次寫回。掉了一次 flush 只會損失 LRU 的精確度，不會掉資料。不可變性 trigger 明確列出內容欄位，因此 `accessed_at` 仍可移動，而讀者能觀察到的一切維持凍結。

容量該設多少取決於大家貼什麼。以下是滿 4000 字元的最壞情況實測：

| 內容            | 原始   | 壓縮後 | 磁碟/筆 | 1 GiB 可放 |
| --------------- | ------ | ------ | ------- | ---------- |
| Go 原始碼       | 4000 B | 250 B  | 360 B   | 3.0M       |
| 結構化／JSON log| 4000 B | ~325 B | 442 B   | 2.4M       |
| 英文散文        | 4000 B | 1048 B | 1434 B  | 749k       |
| 中文散文        | 12 KB  | 3907 B | 4162 B  | 258k       |
| 不可壓縮中文    | 12 KB  | 8734 B | 8937 B  | 120k       |

### 壓縮

貼文頂多幾 KB，而這正是通用壓縮器最吃虧的長度：大半輸入都花在讓壓縮器「認識」這份資料，之後才有辦法便宜地編碼。所以這裡預先準備了一份常見原始碼與 log 片段的字典（[dict/v1.txt](internal/compress/dict/v1.txt)），一開始就把模型餵給它。每筆貼文會同時壓「有字典」和「沒字典」兩種版本，取較小的那個，並**逐列記錄使用的 codec**，因此日後修改字典也不會讓已存資料失效。

演算法與等級都是量出來的，不是猜的。以 160 筆滿版貼文（log、程式碼、散文、不可壓縮資料）實測：

| Codec                   | 字典 | B/筆    | 編碼   | 解碼   |
| ----------------------- | ---- | ------- | ------ | ------ |
| **zstd -19 + 字典**     | ✓    | **760** | 345 µs | 4 µs   |
| brotli q11              |      | 766     | 4.1 ms | 14 µs  |
| zstd -19                |      | 799     | 576 µs | 4 µs   |
| zstd -4 + 字典          | ✓    | 811     | 15 µs  | 4 µs   |
| deflate -9 + 字典       | ✓    | 841     | 99 µs  | 14 µs  |
| gzip -9                 |      | 916     | 82 µs  | 17 µs  |
| xz (LZMA2)              |      | 954     | 577 µs | 211 µs |
| bzip2 -9                |      | 965     | 267 µs | 61 µs  |

bzip2 和 xz 敬陪末座並不意外，只要把輸入大小算進去就懂了：區塊排序與大型 LZMA 視窗都需要遠超過 4 KB 的資料才能回本。level 20–22 在這裡產出與 19 完全相同的位元組，所以預設之上已無空間可爭。

「兩種都壓、取小的」大約值 1% 的總儲存空間，而這兩次壓縮彼此獨立，因此**分在不同核心上跑**。字典那條路有了現成模型，耗時大約只有純壓縮的一半，所以重疊之後拿到的是「較慢的那個」而非兩者之和：位元組完全相同，單筆寫入從 967 µs 降到 661 µs。

剩下的槓桿在字典而非演算法。用貼文**訓練**出來的字典（而非手寫）在保留評估集上量到 705 B/筆 —— 再省 7%，而且超過約 16 KB 的字典就沒有增益了。不過那個數字來自與評估集同源的合成樣本，在真實流量上重現之前，請當成上界看待。

### 儲存

SQLite 在 WAL 模式下允許一個寫入者與多個並行讀取者，所以伺服器就照這個形狀去建模：一條立即取得鎖的單連線寫入端，加上一池釘死在 `query_only` 的讀取連線。讀取完全不碰寫入鎖，也就消除了「共用連線池交錯讀寫」所導致的 database is locked 這類錯誤。

每條連線各自擁有 `HASTE_SQLITE_CACHE_MB` 的 page cache（預設 48 MiB），另有共享的 256 MiB mmap 視窗。只要掃描有實際刪除，就會順便做 WAL checkpoint，確保空間真的被釋放。

寫入需要入場控制，但原因跟直覺不同。SQLite **本身已經是隊列** —— `SetMaxOpenConns(1)` 會把交易 FIFO 序列化，而一次 insert 只要約 100 µs。真正昂貴的是壓縮，而它在交易之前執行、本身沒有任何上限。512 併發實測：放任不管時 p99 296 ms、最差 523 ms；限制成每核心一個寫入、佇列滿了就回 `503`，在相同吞吐下 p99 降到 111 ms、最差 117 ms。

### 編輯器

編輯器是一個透明的 textarea 疊在同一份文字的高光副本上。有兩件事讓它不會散掉：兩層由**單一份**排版度量驅動，而不是兩份會各自漂移的定義；以及只有外層容器捲動，因此不必同步捲動位置，也不會發生「其中一層有捲軸、文字變窄、換行位置就不同」的問題。

上色是在 render 期間**同步**計算的。若改成非同步重繪，可見文字會比游標慢一幀 —— 那看起來就像壞掉。

## 測試

```bash
make test
```

Go 測試涵蓋真正重要的不變量：跨層級的分享碼唯一性、每一層確實是一個置換、分享碼遵守最短長度且不洩漏順序、不可變性 trigger、讀取池拒絕寫入、pragma 確實套用到兩個連線池、**空間上限在每一次寫入都成立**、淘汰時移除的是最久沒被**讀取**的而非單純最舊的、兩個 TTL（含「預設關閉」這件事本身）、讀取在 flush 前不會寫入、寫入佇列滿時會拒絕、併發建立不重複、各語言的下載檔名，以及完整的 HTTP 介面（含各項限制與 raw 端點的防護標頭）。

有兩組測試是「報告」而非「斷言」，因為它們量的是所在機器：`TestStorageFootprint` 印出各類內容滿版貼文的實際磁碟成本，`TestLevelTradeoff` 印出每個 zstd 等級的大小與時間。它們支持的結論則由旁邊的一般斷言鎖住。

前端測試守住語言偵測 —— 那是一堆啟發式規則，而啟發式最容易**悄悄退化**：為某個語言放寬的規則，會默默搶走另一個語言的貼文。[languages.test.ts](web/src/lib/languages.test.ts) 裡的每一個案例都來自真實的誤判，所以這份語料庫只會增加、不會刪減。[lines.test.ts](web/src/lib/lines.test.ts) 則涵蓋 fragment 解析與範圍選取，包含 shift 往上點所產生的反向範圍。

## 專案結構

```
cmd/haste/          進入點、graceful shutdown、保留策略掃描器
internal/config/    .env 載入與驗證
internal/id/        計數器轉短碼（分層 + Feistel 置換）
internal/compress/  zstd codec 與內嵌字典
internal/store/     SQLite schema、讀寫連線池、查詢
internal/httpapi/   路由、middleware、流量限制、SPA 服務
internal/webui/     內嵌的前端建置產物
web/                React + Tailwind + shadcn/ui 原始碼
```
