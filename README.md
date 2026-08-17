# haste-server

*中文 · [English](README-EN.md)*

貼上程式碼或 log，拿到一個短分享碼。單一 Go binary：JSON API、raw 端點、React 前端全部內嵌其中。

- **短 hash 風格分享碼，且結構上不可能碰撞** —— `k7Qm2Xp9`，不是 `1`、`2`、`3`。
- **寫入即鎖死。** 沒有編輯或刪除路徑，且由資料庫本身強制執行。
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
  "createdAt": "2026-08-18T16:19:25Z",
  "expiresAt": "2026-09-17T16:19:25Z"
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

錯誤一律回 `{"error": "code", "message": "..."}` 並搭配對應狀態碼：`400` 空白或格式錯誤、`413` 超過上限、`429` 觸發流量限制、`404` 找不到或已過期。

## 設定

所有設定都在 `.env`（註解完整的清單見 [.env.example](.env.example)）。實際的環境變數優先於檔案內容。

| 變數                     | 預設值           | 說明                                    |
| ------------------------ | ---------------- | --------------------------------------- |
| `HASTE_ADDR`             | `:8080`          | 監聽位址。                              |
| `HASTE_MAX_CHARS`        | `4000`           | 算 Unicode 字元，不是 bytes。           |
| `HASTE_CODE_MIN_LEN`     | `8`              | 分享碼最短長度，1–10 個 base62 字元。   |
| `HASTE_RETENTION`        | `30d`            | 接受 `d`、`w`；設 `0` 表示永久保留。    |
| `HASTE_CLEANUP_INTERVAL` | `1h`             | 清理過期資料的間隔。                    |
| `HASTE_ZSTD_LEVEL`       | `19`             | 1–22。                                  |
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

貼文寫入即鎖死。沒有更新或刪除端點，而且即使有寫入真的抵達資料表，`BEFORE UPDATE` trigger 也會直接中止 —— 所以未來任何程式路徑、migration，或是有人開 `sqlite3` 進去，都無法悄悄改寫一個已經分享出去的碼。只有過期才會刪除。

### 行號連結

行號是用 CSS counter 畫出來的，不是文字，因此永遠不會被拖進選取範圍、跟著程式碼一起被複製。但這樣就沒有東西可以點，所以每一行另外掛了一個**空的** anchor 疊在行號區上：可點、可連結，本身沒有任何文字會跑到剪貼簿。

選取狀態只存在於 URL fragment，沒有第二份狀態 —— 網址列看到的連結，永遠就是對方會收到的連結。

### 壓縮

貼文頂多幾 KB，而這正是純 zstd 最吃虧的長度：大半輸入都花在讓壓縮器「認識」這份資料，之後才有辦法便宜地編碼。所以這裡預先準備了一份常見原始碼與 log 片段的字典（[dict/v1.txt](internal/compress/dict/v1.txt)），一開始就把模型餵給它。

每筆貼文會同時壓兩種版本，取較小的那個，並**逐列記錄使用的 codec**，因此日後修改字典也不會讓已存資料失效。本版實測：

| 輸入                | 原始  | 儲存  | 比率  |
| ------------------- | ----- | ----- | ----- |
| 三行結構化 log      | 301 B | 19 B  | 15.8× |
| 單行 JSON log       | 111 B | 20 B  | 5.6×  |
| 小型 Go 程式        | 66 B  | 45 B  | 1.5×  |

### 儲存

SQLite 在 WAL 模式下允許一個寫入者與多個並行讀取者，所以伺服器就照這個形狀去建模：一條立即取得鎖的單連線寫入端，加上一池釘死在 `query_only` 的讀取連線。讀取完全不碰寫入鎖，也就消除了「共用連線池交錯讀寫」所導致的 database is locked 這類錯誤。

每條連線各自擁有 `HASTE_SQLITE_CACHE_MB` 的 page cache（預設 48 MiB），另有共享的 256 MiB mmap 視窗。過期資料每小時清理一次；只要有實際刪除，就會順便做 WAL checkpoint，確保空間真的被釋放。

### 編輯器

編輯器是一個透明的 textarea 疊在同一份文字的高光副本上。有兩件事讓它不會散掉：兩層由**單一份**排版度量驅動，而不是兩份會各自漂移的定義；以及只有外層容器捲動，因此不必同步捲動位置，也不會發生「其中一層有捲軸、文字變窄、換行位置就不同」的問題。

上色是在 render 期間**同步**計算的。若改成非同步重繪，可見文字會比游標慢一幀 —— 那看起來就像壞掉。

## 測試

```bash
make test
```

Go 測試涵蓋真正重要的不變量：跨層級的分享碼唯一性、每一層確實是一個置換、分享碼遵守最短長度且不洩漏順序、不可變性 trigger、讀取池拒絕寫入、pragma 確實套用到兩個連線池、過期與清理、併發建立不重複、各語言的下載檔名，以及完整的 HTTP 介面（含各項限制與 raw 端點的防護標頭）。

前端測試守住語言偵測 —— 那是一堆啟發式規則，而啟發式最容易**悄悄退化**：為某個語言放寬的規則，會默默搶走另一個語言的貼文。[languages.test.ts](web/src/lib/languages.test.ts) 裡的每一個案例都來自真實的誤判，所以這份語料庫只會增加、不會刪減。[lines.test.ts](web/src/lib/lines.test.ts) 則涵蓋 fragment 解析與範圍選取，包含 shift 往上點所產生的反向範圍。

## 專案結構

```
cmd/haste/          進入點、graceful shutdown、過期清理器
internal/config/    .env 載入與驗證
internal/id/        計數器轉短碼（分層 + Feistel 置換）
internal/compress/  zstd codec 與內嵌字典
internal/store/     SQLite schema、讀寫連線池、查詢
internal/httpapi/   路由、middleware、流量限制、SPA 服務
internal/webui/     內嵌的前端建置產物
web/                React + Tailwind + shadcn/ui 原始碼
```
