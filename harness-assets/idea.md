# gitreu

> 照 SourceTree 的畫面安排做的 git TUI：左邊是分支列表，右邊是分支圖，SourceTree 的右鍵選單就用 Space 選單來呈現。terminu 家族的一員。

## 要解決的問題

- **user 的經驗**：用 lazygit 不順手。它把操作拆成一堆熱鍵組合，看起來很潮，但違反 user 對 TUI 的想法：每次都要按 `?` 或上網查自己要的功能怎麼做，最後乾脆直接打開 SourceTree。仿 Magit 的 gitu 也不順手。
- **跑回 SourceTree 的時候，都是要做這三件事**：
  1. 看分支圖，可以切換「全部分支」和「目前分支」。
  2. merge 和解衝突。
  3. 比較兩個 commit：有哪些檔案變了，再進去看每個檔案前後的差異。
- **目標使用者**：在終端機工作、不想背熱鍵的人。第一個使用者是 user 自己。
- **成功的標準**：做出來之後，不再打開 SourceTree。

## 核心功能（MVP）

- [ ] 開啟時的畫面：左右兩個面板，左邊是分支列表，右邊是分支圖，跟 SourceTree 一樣。
- [ ] 分支圖可以切換「全部分支」和「目前分支」。
- [ ] 一般的 git 操作都在 app 裡完成（stage、commit、push、pull、fetch、開分支、merge、stash、tag 等），全部從 Space 選單執行。SourceTree 右鍵選單上的動作，都對應到 Space 選單。
- [ ] merge 和解衝突。
- [ ] 比較兩個 commit：選兩個 commit，列出有變動的檔案；在檔案上按 Enter，看它前後的差異；按 Esc 一層一層退回。

## 技術選擇

- **語言與框架**：Go、terminu-go，遵循 tdp。
- **開發方式**：用 terminu-harness（`/terminu`）從需求一路做到 release。
- **發佈**：比照家族其他成員，GitHub Releases 和 Homebrew tap（`vulcanshen/tap`）。
- **怎麼操作 git**：還沒討論，是呼叫 git 指令，還是用 Go 的 git 函式庫。

## 架構概述

面板、Space 選單、`?` 按鍵說明、Esc 一層一層退回、popup 這些，tdp 和 terminu-go 都已經有了。user 的判斷是：app 只要決定開啟時的畫面怎麼安排，其餘交給 tdp 和 terminu-go，自己專注在 git 的內容。這跟 tetru 的情況一樣，框架固定了，只要專心做 app 本身。

## 已知風險與待釐清事項

- **GUI 搬到 TUI 會有水土不服的地方**：SourceTree 畢竟是 GUI，哪裡會不合，要做出來實際用過才知道。user 決定先把 app 做出來，再回頭打磨。tdp 沒寫到的，照家族的流程回 tdp 開 issue。
- **分支圖**：要在終端機裡把分支線畫得清楚又好看，大 repo 裡也要夠快。
- **解衝突**：三件事裡面最難的。每個衝突的區塊都要選「用我的」「用對方的」或「兩個都要」，畫面得同時看得到兩邊。
  - 可能會用到 tdp「app 自己的模式」（K11）。到目前只有 tetru 用過 K11，gitreu 會是第二個，正好可以驗證那些條文是家族共通的規則，還是只有遊戲才需要。
- **比較兩個 commit**：tdp 已經規定，裝著東西的項目一次只走一層（panel.10），就像 filu 走資料夾那樣。commit → 檔案 → 差異，應該照這條走。
- **怎麼操作 git**：呼叫 git 指令還是用 Go 的 git 函式庫，還沒決定。

## 第一版看過之後（2026-11-19）

user 看了第一版的回饋，照原話：

1. 「[F]ile status 命名是不是有點怪？還是這是慣例，這部份不是 working tree 嗎？[H]istory tab 裡面有個 History panel 這樣命名怪怪的吧」
2. 「branches panel 裡面有個 gone 是啥意思？」（看不懂就是沒寫清楚）
3. 「graph 的 commit 點有沒有其他 glyph 可以用，圓點有點單調」
4. 「panel 1 的 enter 不要 checkout，換成切換 focus 到 history panel 然後 scroll 到該 head 的 commit 點位置」
5. 「header status 中的 Repo 和 Branch 交換位置，然後 Repo value 也要上色」

其他的下一波再說。

參考：GitKraken（user：「可以參考 gitkraken，這款我因為要付費所以沒用的超級酷的 git GUI」）。

## branches panel（2026-11-19）

user 照原話：

1. 「check out 熱鍵用 c」
2. 「rename 熱鍵用 r」
3. 「REMOTE 和 TAG 用分頁作，不要都擠在一個 panel」
4. 「history [1] panel：R[e]base -> Re[b]ase」

## history [2] commits（2026-11-19）

user 照原話：

1. 「graph 的線似乎太多了，分支好多，sourcetree 和 gitkraken 有處理這個問題嗎？」（先回答、再決定要做什麼）
2. 「需要可以 zoom，但是 zoom 分兩段，第一段是還保留 panel 1 最小寬度，第二段是完全展開，panel 1 就完全被覆蓋，當窄寬時，就不會有 panel 1 最小寬度那一段」

看了 SourceTree、GitKraken 的做法之後（2026-11-19），user 照原話：

1. 「我認為需要像 sourcetree 那樣有 show all/current 的展示，但是 sourcetree 裡面這兩個差異只是主線是誰，current 的時候，當前 head 一定是在最左邊，而 all 的時候應該是以 main/master 在最左邊，或者沒有主線」（追問後：All 與 Current 都畫全部的分支，只差最左邊的主線）
2. 「我不懂 hide/solo 的顯示方式」（解釋後：先不要）
3. 「顯示/隱藏 remote 確實是個好方法，畢竟正常開發只會關注自己的分支」（追問後：預設隱藏）
4. 「tag 應該要在 commit panel 中，但現在欄位似乎有點緊」（追問後：tag 用膠囊顯示；膠囊是 tdp 保留給名稱的，先用 glyph，同時回報 tdp）
5. 「那 upstream gone 也加個 glyph」

## 跟上 repo 的變動（2026-11-19）

user 照原話：

1. 「我沒有看過 work tree 的樣子，work tree 都是空的，然後我們多久 resync repo 一次？還是我們得用 watch？」（回答：每 3 秒重新讀一次；說明 watch 的做法與代價）
2. 「watch 工很多嗎？」
3. 每 3 秒的背景重讀要不要記進 App Log（review 判 tdp 不清楚）：「那就用 watch 啊，這樣就可以記」；重讀的方式選了「完整的 watch」
4. 做了 watch 之後聊它的做法與困難點（平台的監看上限、分不出是誰改的、編輯器暫存檔的雜訊）：「那如果換成定時刷新呢？」（回答：沒有平台限制、自己的操作不會被誤記，每次刷新跟上一次比、有變才記；代價是最慢 3 秒才跟上）
5. 「ok 改定時刷新，3 秒」

## 後續可擴充

- 開工之後，依實際使用時發現的 GUI 和 TUI 不合的地方，回頭打磨細節。

## Brainstorm 筆記

2026-11-19 的討論：

- **為什麼 SourceTree 的設計比較符合 terminu**：
  - lazygit 主要靠背每個面板各自的單鍵，動作散在各個面板的按鍵裡，左邊五、六個小面板疊在一起。
  - SourceTree 在東西上按右鍵，能做的全部列出來。右鍵選單管「這個 commit、這個分支」，上方工具列管「整個 repo」，左邊是側欄，中間是 commit 圖，下面是檔案和差異。
  - 對到 terminu：右鍵選單就是 Space，工具列就是作用在整個 app 的那一層動作，各個區塊就是面板。lazygit 卡住的地方，正好是 terminu 的第一條精神「不用先背好才知道能做什麼」。
- **gitu 為什麼有 3,000 多顆星**：星星大多來自 Magit 的使用者。Magit 那一派是「先記住入口鍵，再從跳出來的選單挑」，習慣 Emacs 的人用起來順手，但跟「不用背」是兩回事。
- **user 跑回 SourceTree 的那三件事，都是「看」的工作**：git 指令什麼都做得到，終端機缺的是能把歷史長什麼樣子、兩邊差在哪裡攤開來看的畫面。
- **範圍**：一開始我問 stage、commit、push 要不要放進來，user 決定一般的 git 操作全部都要進到這個 app，透過 Space 選單操作。
- **開發順序**：先照 SourceTree 的安排把 app 做出來，水土不服的地方之後再打磨。
- **名字**：
  - gitu 已經被用掉了：altsem/gitu，仿 Magit 的 git TUI，大約 3,000 顆星。另外還有 gitui，大約 2.2 萬顆星。
  - 我提的 branchu、twigu、mergu、grovu，user 都覺得不好。
  - gittu 沒有撞名，但多出來的 t 只是為了跟 gitu 區隔。
  - gitru 撞名：ruru-m07/gitru 是一個用 Tauri 做的 git 客戶端，291 顆星，有自己的網站 gitru.app。
  - 最後定為 **gitreu**：git 和 tree 共用一個 t，再接上家族的 u。tree 指的是 SourceTree，也是右邊那個分支圖。做法跟 Alphade 讓 alpha 和 fade 共用一個 a 一樣。
  - 撞名檢查（2026-11-19，只查了 GitHub 和 Homebrew）：GitHub 上只有一個 2024 年開的同名 repo（DiegoDiMattina/gitreu），裡面只有 1KB，開了當天之後就沒再動過，0 顆星；Homebrew 沒有。
