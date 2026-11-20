# gittu（原名 gitreu）

> 終端機的 git TUI：左邊是分支列表，右邊是分支圖，能做的事都在 Space 選單裡。terminu 家族的一員。

## 要解決的問題

- **user 的經驗**：終端機的 git 工具常把操作拆成一堆熱鍵組合，違反 user 對 TUI 的想法：每次都要按 `?` 或上網查自己要的功能怎麼做，最後乾脆改開 GUI。
- **改開 GUI 的時候，都是要做這三件事**：
  1. 看分支圖，可以切換「全部分支」和「目前分支」。
  2. merge 和解衝突。
  3. 比較兩個 commit：有哪些檔案變了，再進去看每個檔案前後的差異。
- **目標使用者**：在終端機工作、不想背熱鍵的人。第一個使用者是 user 自己。
- **成功的標準**：做出來之後，不用再另外開 GUI 的 git 工具。

## 核心功能（MVP）

- [ ] 開啟時的畫面：左右兩個面板，左邊是分支列表，右邊是分支圖。
- [ ] 分支圖可以切換「全部分支」和「目前分支」。
- [ ] 一般的 git 操作都在 app 裡完成（stage、commit、push、pull、fetch、開分支、merge、stash、tag 等），全部從 Space 選單執行。對分支、遠端分支、tag、stash、commit 能做的事，都在 Space 選單裡。
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

- **GUI 的習慣搬到 TUI 會有水土不服的地方**：哪裡會不合，要做出來實際用過才知道。user 決定先把 app 做出來，再回頭打磨。tdp 沒寫到的，照家族的流程回 tdp 開 issue。
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


## branches panel（2026-11-19）

user 照原話：

1. 「check out 熱鍵用 c」
2. 「rename 熱鍵用 r」
3. 「REMOTE 和 TAG 用分頁作，不要都擠在一個 panel」
4. 「history [1] panel：R[e]base -> Re[b]ase」

## history [2] commits（2026-11-19）

user 照原話：

1. 「graph 的線似乎太多了，分支好多」，問有沒有辦法處理（先回答、再決定要做什麼）
2. 「需要可以 zoom，但是 zoom 分兩段，第一段是還保留 panel 1 最小寬度，第二段是完全展開，panel 1 就完全被覆蓋，當窄寬時，就不會有 panel 1 最小寬度那一段」

討論過分支圖的畫法之後（2026-11-19），user：

1. 要有 show all/current 的切換，兩者的差異只是主線是誰：「current 的時候，當前 head 一定是在最左邊，而 all 的時候應該是以 main/master 在最左邊，或者沒有主線」（追問後：All 與 Current 都畫全部的分支，只差最左邊的主線）
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

## 改名（2026-11-19）

user 照原話：

1. 刪掉 GitHub 上的 repo：「我想要先撤掉 remote repo，想要改名」
2. 「我們可以現在改名，再 push」
3. 新名字：「gittu: git terminu ui，icon 你不用管，我會用 kamonu 作」；新的 repo 公開

## 篩選（2026-11-19）

在分支上按 Enter、[2] 的篩選擋住那個 commit 時要不要清掉篩選（review 判 tdp 不清楚），user 照原話：「對，我搞不清楚你的篩選是在說什麼」。
說明之後，user 在 tdp session 選了 A：app 自己的動作要換掉 panel 列的東西、或把 cursor 放到某一項上時，先清掉那個 panel 的篩選
（不管有沒有擋住那一項）；只是 focus 離開不清。tdp main 7e62a11 寫進 panel.14（checklist panel-180），還沒發版。

## 背景刷新的紀錄與沒追蹤的資料夾（2026-11-19）

review 判 tdp 不清楚的另外三題，user 先問了「repo 壞掉？是怎麼壞掉？」、「沒有追蹤的整個資料夾，啊就沒有追蹤了，顯示什麼？」，
說明之後在 tdp session 都選了 A（tdp main，還沒發版）：

1. 自己重讀的內容，讀到變了才記一筆 INFO 寫出變了什麼，沒變不記（note.7，note-104）
2. 自己重讀失敗：第一次跳 Error toast、記一筆 ERR，同一個失敗再發生不跳不記，換一種再跳再記，又讀得到時記一筆 INFO、不跳 toast（note.7，note-105）
3. 明細 panel 顯示裝著許多項目的東西（沒追蹤的資料夾、commit）時，照 panel.9 列出有差異的項目、各有標記；沒追蹤的資料夾裡的檔都是 `+`；
   Enter 進到那份清單（note.8，note-106）。terminu-go 要新的 API，v1.2.19-beta.2 才有

## commits panel 與比較的畫面（2026-11-19）

user 照原話（附了一張比較的截圖：標題太長，UNIFIED／SPLIT 被截成 …）：

1. 「commits panel：R[e]base 移除熱鍵，這種操作不要給熱鍵誤觸」
2. 「Re[s]et 同 (1)」
3. 「Show [r]emote branches 同上」
4. 「diff 這種 title，根本看不到 unified/split tab，那個不應該放在 border title 上吧？」
5. 「diff 的 popup 不夠大」
6. 「然後我們要添加 pull all（sync）這種 global operation（或這歸類到 history 的 panel operation）」
   （問做什麼時：「就是 pull --all 的 git 指令啊？」；放哪裡照 tdp P3：作用在整個 repo 是 global operation）

## commit 的 ref 放第二列（2026-11-19）

user 照原話：「我發現如果在 description 裡面放 branch head 和 tag，會讓 description 不夠放，我們是否可以讓有這些 tag 描述的
commit 兩列，第二列就顯示這些 tag，cursor 移動的時候，focus 也只會移動在 commit 點的那一列，但是第二列（如果有了話），
會連帶亮一點背景色」

## trace（2026-11-20）

user 照原話：「我想要在 commit panel 有一個操作是『trace』，分別是大寫 T 和小寫 t，或者大寫 N 和小寫 n（那就變 next）的概念，
功能是當 commits panel 是 current branches 的時候，才有這個功能，可以按熱鍵，直接跳去當前分支的上一個或下一個 commit 點」

追問後：熱鍵用 n 與 N（n 往下、較舊；N 往上、較新）；當前分支的 commit 是 HEAD 走得到的所有 commit（merge 進來的也算）；
cursor 不在那些 commit 上時，往按的方向跳到最近的一個。

## commits 的 Current／All 分頁（2026-11-20）

user 先要「SETTING 分頁，現在有一個使用者設定，就是 commits panel 預設顯示『All branches』還是『Current branch』」，
以及「當 commit panel 預設顯示 current branch 時，當 cursor 在左邊 panel 移動切換 branch 的時候，右邊 commit panel 也就會跟著切換」。
接著改成，user 照原話：

1. 「把上方 Branch: <current branch>, Repo: <repo> 的顯示，改放到 panel 1 上方，但是不給 focus，不給 focus 的功能你可以參考 tetru，這樣上方可以空出來」
   （tdp 沒有不拿 focus 的框，要先請 tdp 定；user：「等等讓我想一下」）
2. 「我們的 panel 2 也多一個分頁，COMMITS 有分 Current 和 All 兩個分頁，預設是 Current，使用者切換到哪一個，就固定在那一個，
   兩個分頁都會依照左方 panel 1 切換 branch 而切換內容」
3. 「這樣 Settings 就先不需要」

先做 2：「先作 COMMIT current/all 分頁，關掉在打開不記得，只記到這次關掉為止」

## 放大時分支圖變寬（2026-11-20）

user 照原話（附了一張放大到整個畫面、線被截成 … 的截圖）：「panel 2 在 zoom 的時候，是不是 GRAPH 區域可以動態放大」
追問後選了：照 [2] 的寬度隨時調（終端機越寬、放大越多，線就畫越多；窄的時候至少還是 10 條；DESCRIPTION、AUTHOR、DATE 不會被擠窄）

## 一個 commit 的檔案放在常駐的 [3]（2026-11-20）

user 照原話：「我認為 commit 點按下 enter drill into 看到檔案清單這個體驗有點怪怪 1. 上方 tab 還是顯示 current/all, 但是這時候切換 h|l 不會有任何差異，因為已經進入某一個 commit 的 detail 2. 改成按下跳出 commit detail popup 對順序的體驗是對的，但是對操作的體驗又怪怪的」
給了三個做法（A：[2] 下面加一個明細 panel [3]，跟著 cursor 列那個 commit 的檔案；B：進到清單時把分頁藏起來；C：popup）後：「我認為 A 不錯，但是 panel 3 這樣會變成常駐，我又不想要那塊空間是常駐的」
再給「按 Enter 才開、Esc 收起」與「一個鍵切換開關」兩種後：「我們先讓 panel 3 常駐吧」

## checkout 前警告工作區的改動（2026-11-20）

user 照原話：「我發現當 working tree 有東西的時候，切換（checkout）branch 應該要跳 confirm 警告」
追問後：確定之後照 git 的做法 checkout（改動帶過去，git 拒絕時跳錯誤）；算 staged 與 unstaged 的改動，沒追蹤的檔不算；
[1] 的本地分支、遠端分支、tag 與 [2] 的 commit 都要。

## 不寫別的工具（2026-11-19）

user 照原話：「整個 repo，把其他工具的描述都移除，我只試用那些工具來描述給你聽而已，不是真的要照他們的樣子」

- 紀錄、README、程式（說明文字、splash、Homebrew 的說明、註解）都只寫 gittu 自己，不拿別的 git 工具比；
  上面原話裡拿別的工具舉例的地方，改成只寫要的行為。已經 push 的 commit 訊息不改寫。

## 後續可擴充

- 開工之後，依實際使用時發現的 GUI 和 TUI 不合的地方，回頭打磨細節。

## Brainstorm 筆記

2026-11-19 的討論：

- **為什麼這樣安排符合 terminu**：在東西上按 Space，能做的全部列出來：item operation 管「這個 commit、這個分支」，
  global operation 管「整個 repo」，左邊是分支，右邊是 commit 圖。不用先背熱鍵，正是 terminu 的第一條精神「不用先背好才知道能做什麼」。
- **那三件事都是「看」的工作**：git 指令什麼都做得到，終端機缺的是能把歷史長什麼樣子、兩邊差在哪裡攤開來看的畫面。
- **範圍**：一開始我問 stage、commit、push 要不要放進來，user 決定一般的 git 操作全部都要進到這個 app，透過 Space 選單操作。
- **開發順序**：先把 app 做出來，水土不服的地方之後再打磨。
- **名字**：先定為 gitreu（git 和 tree 共用一個 t，再接上家族的 u；tree 是右邊那個分支圖），2026-11-19 改名為 **gittu**
  （git terminu ui，見「改名」）。撞名檢查（只查 GitHub 與 Homebrew）：GitHub 沒有 vulcanshen/gittu，Homebrew 沒有 gittu。
