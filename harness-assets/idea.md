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

## Working tree 的檔案可以複選、一起操作（2026-11-20）

user 照原話：「我們先作 working tree 那邊的調整 1. 我要 changes 清單的 item 可以複選，然後可以一起進行操作，例如 resolve conflict use mine/theirs, discard, remove, add to ignore... 等等的」
問了標記鍵、remove 與 ignore 的細節後：「我不懂你的問題是什麼，不就是 checkbox 勾選 -> space menu 操作嗎?」（細節由 app 決定）

## 衝突的行號寫成範圍（2026-11-20）

提示原本寫 lines 2, 6, 14, 18（兩塊衝突的頭尾），列了改成範圍 lines 2–6, 14–18 與維持現狀之後，user 照原話：「1」

## 80×24 也能開（2026-11-20）

實測發現 80×24 的終端機只看到最低尺寸的提示，列了支援到 24 列與維持 32 列之後，user 照原話：「1」（支援到 24 列）

## statusbar 放不下時的優先順序（2026-11-20）

實測發現 60 欄時 merge 的狀態整個不見，user 照原話：「優先級: 狀態 > 分支 > repo」

## 長路徑留著狀態字母（2026-11-20）

實測發現長路徑連前面的狀態字母一起截掉（D、? 分不出來），列了 gittu 自己截路徑、請 tdp／terminu-go 支援沒有表頭的兩欄、維持現狀三種後，user 照原話：「1」（gittu 自己截路徑，字母留著）

## 改過的衝突檔，整個用一邊之前先問（2026-11-20）

實測發現整個檔的 Use mine／Use theirs 會蓋掉已經一塊一塊選好的部分，user 照原話：「C5，我不太懂什麼整個檔會蓋掉已經做的選擇？」
「謂的「改過才問」，意思是會把使用者有手動調整過的檔案，但還沒有標 resolved 的先標記起來，如果之後有備使用者進行 use theirs/mine 的操作的時候，就先跳出警告?」
「但是如果gittu關閉重開，還會記得嗎?」
說明改成比檔案的修改時間（衝突之後存過檔就算改過，重開 gittu、在 gittu 外面改的也算；stash 套用的衝突比不出來，每次都問）之後：「好，照這樣做」

## gittu 的 editor 開著時不刷新（2026-11-20）

實測發現用 gittu 開的 vi 改衝突的檔時，App Log 記了 Changed outside gittu: .conflict.txt.swp 與那個檔，列了 editor 開著時先不刷新（關掉後本來就會重新讀）與維持現狀之後，user 照原話：「1」

## 打 tag 的 toast 寫 as（2026-11-20）

實測記下打 tag 成功的 toast 寫 Tagged 49ee96e v0.2.0，列了改成 Tagged 49ee96e as v0.2.0 與維持現狀之後，user 照原話：「1」

## Abort 的確認寫清楚會回到哪裡（2026-11-20）

實測記下 Abort 的 confirm 寫 What it did so far is undone, conflicts resolved too.（像是衝突會被解掉），列了改成 Everything goes back to how it was before the merge; conflicts you have resolved are lost. 與維持現狀之後，user 照原話：「1」

## 遠端分支：沒有本地的就 checkout，有本地的照樣畫（2026-11-20）

實測記下 [1] REMOTES 的 Enter 把 [2] 跳到一個 commit，可是遠端分支預設不畫、看不出為什麼跳到那裡；給了跳 toast 說明與維持現狀兩種之後，user 照原話：
「1. 如果 local 沒有 checkout 該 remote branch, 那就先 checkout 到 local, 然後切過去
2. 如果 local 有這個 branch, 我們也應該標這個 branch 的 remote branch, 通常線會是同一條線，頂多往前多走幾個點」

## [4] 寬一點，[3] 可以放大（2026-11-20）

實測記下 60 欄時 [4] 的位置列 f78f69a ↔ 49ee96e 被從前面截掉，列了維持現狀與回報 tdp 之後，user 照原話：「我們可以增加 [4] 的寬佔比，減少 [3] 的寬佔比，給[3] zoom 效果，反正 [3] 只是拿來看」

## 沒追蹤的檔，左邊寫 empty（2026-11-20）

實測記下 Working tree 裡還沒追蹤的檔，[2] 寫 brand-new.txt @ index ↔ brand-new.txt（index 裡根本沒有它），列了改成 @ empty 與維持現狀之後，user 照原話：「1」

## 沒有改動時 [2] 不叫你去 [1]（2026-11-20）

實測記下什麼都沒改時 [2] 寫 No file to show / Press [1] to go to the files（[1] 根本沒有檔），列了改成跟 [1] 一樣的 Press [H] to see the history 與維持現狀之後，user 照原話：「1」

## 使用者自訂的動作（2026-11-20，下一版）

user 照原話（舉了另一個 git 工具當例子，照不提其他工具的約定不寫名字）：「我有想添加一個功能，就是……讓使用者客製化action, 我們用 ${{source}} 當成來源分支，${{target}} 當成目標分支，讓使用者自己開發自己的操作, 腳本要存起來, 這應該算 data, 不算 gittu 的 config, 應該有第三個 tab, CUSTOM 來放這些東西，但是我們應該可以先發版」

## Mark [r]esolved（2026-11-20）

tdp 開放框的位置之後，問 [r]esolved 要不要改，user 照原話：「改成 Mark [r]esolved」

## tdp 開放了框的位置：改回 Stage [a]ll（2026-11-20）

tdp main 3f82387 照 user 的要求改了 M5（M5-1）：預設框第一次出現的字母，app 可以挑別的。user 原本要的是「toggle stage [a]ll」，所以 a 的兩個 label 改回 Stage [a]ll／Unstage [a]ll

## a 切換全部 stage／unstage，寫成 [a]ll staged（2026-11-20）

user 照原話：「st[a]ge all -> toggle stage [a]ll」；說了 tdp M5 把鍵框在 label 第一次出現的字母上、Stage all 一定畫成 St[a]ge all 之後，選第 1 種：label 改成 a 開頭的 [a]ll staged／[a]ll unstaged
接著：「是說這一點回饋給 tdp 吧，這點請 tdp 開放 app 想要[]在哪就框在哪」

## Diff 模式裡 [3] 只跟著 cursor（2026-11-20）

問 Diff 模式中 cursor 移動時 [3] 顯示什麼（只有 cursor 那個 commit 的訊息），提了列出兩者之間的 commit、或上下放兩邊的訊息之後，user 照原話：「我認為應該只跟著 cursor 的 description」

## [3] 拆成左右：左邊 description、右邊檔案（2026-11-20）

user 照原話：「[3]Files 拆成左右兩個區域，左邊顯示 description, 右邊在顯示 files list」

## statusbar 的順序：狀態、Branch、Repo；M 再按一次是 Unmark all（2026-11-20）

看了 statusbar 中間一大段空白（「首先, status bar 位置很怪」），user 照原話：「狀態|Branch|Repo <- 順序」
「然後既然 [M] 是 mark all, 那在按一次 M 應該是 unmark all」

## merge 的狀態 icon、commit 改成 form（2026-11-20）

user 照原話：「如果 merge conflict 發生中，status bar 應該要有一個狀態 icon 才對，顯示當前狀態，然後 commit 的輸入視窗太糟糕了，這部份我們得切個 form 出來，第一 row 是 title, 第二 row 是 textarea 第三是一個 Submit buttom」

## 標成已解決的那一項寫成 [r]esolved（2026-11-20）

Space menu 裡的 Mark resolved 照 tdp M5 畫成 Ma[r]k resolved，user 照原話：「ma[r]k resolved 那個 [] 位置很怪吧，應該是 [r]esolved  mark as resolved...」

## 標記的檔標成已解決沒有熱鍵（2026-11-20）

說了 R 也是全域的 Refresh 之後，user 照原話：「什麼時候有全域 Refresh, 使用者沒有被揭露，如果這樣那 [R]esolve 全部這個熱鍵就取消, 一樣用 mark 多筆或單筆 resovle」

## 衝突用 editor 解，自己標成已解決（2026-11-20）

user 試了 demo 的衝突後，照原話：「然後現在解決 conflict 的方式很不行，這樣根本沒有解決，我們應該是按下 enter 打開 editor, 讓使用者編輯, 編輯結束後 reload, 並幫使用者檢查是否還有衝突的標注區域, 然後要有 mark as [r]esolved (單一檔)的功能, [R] 就是多選後 mark as resolved 所有勾選檔案」
接著：「我們的ux 應該是，在 conflict 的 item 上按下 enter -> focus 到 panel 2(diff), 然後在按下 enter -> 開啟 editor」

## 一個檔的 Discard 沒有熱鍵（2026-11-20）

user 照原話：「discard 不要用 x, 一樣沒有熱鍵」

## 整個檔的 Use mine、Use theirs 沒有熱鍵，Mark all 用 M（2026-11-20）

user 照原話：「Use mine 沒有熱鍵, Use theirs 也移除, [m]ark 單一筆, [M]ark all 大寫M」

## Diff 模式的結果放在 [3]（2026-11-20）

user 照原話：「關於剛剛那個兩個 commit 點 diff 的情況，我想要的是，當進入 diff mode 之後，panel 3 的內容就直接跟著 cursor 移動對兩個 commit 進行 diff, 顯示內容，也就是說 panel 3 的邏輯是，當只有一個 commit 被 focus 的時候，就顯示那一個 commit 的 detail 內容，當有兩個 commit 備 focus (diff mode) 的時候，就顯示那兩個 commit 點的 diff 內容, 這樣 panel 2 diff mode 按下 enter 的時候，就直接 focus 到 panel 3 就好，但這問題是，這時候 panel 3 應該要要有 diff mode 的狀態，然後 panel 2 應該也是保持 diff 2, 這還需要考慮」
「那我們來履一下，當 panel 2 進入 diff mode 的時候，同時 panel 3 應該也要一起進入 diff mode 並且亮起來 然後當 panel 2 確定第二個 commit 點的時候按下 enter, focus 直接切換到 panel 3, 但是因為 panel 2 和 panel 3 都是亮著，那使用者怎麼知道當前 focus 在哪裡? 但如果不一起亮，又很難知道是連動的」（用 tdp L5 的框線粗細分 focus 後接受）
「ok 既然這樣那就得在考慮操作，因為 esc 會離開 mode, 但是 panel 3 在 diff mode 之中的 esc, 應該是 focus 回 panel 2 才對，因為使用者操作的整個 breadcrumb 是這樣的，所以 esc 應該一樣的回去」
「ok 送給 tdp session」

## Discard all 不要熱鍵（2026-11-20）

照 tdp K13 把 Discard all 放到 X 之後，user 照原話：「Discard all 不要熱鍵」

## 標記用 m，Diff 模式用 D（2026-11-20）

user 照原話：「mark 熱鍵用 [m] 吧, 然後 diff mode 熱鍵用 [D]」
（另外問 CHANGES 拆兩欄會多一列 FILE 欄名時，選回一欄：「1，回報給 terminu-go 修截斷」；查了之後 terminu-go 沒有截錯，長路徑只是連前面的狀態字母一起從前面截）

## 欄寬：DESCRIPTION 吃掉剩下的寬度（2026-11-20）

user 照原話（附了一張放大後欄位擠在左半邊、右半邊空著的截圖）：「是可以了，但是可以平均點嗎?」
選了「DESCRIPTION 吃掉剩下的寬度」（AUTHOR、DATE 靠右）；tdp session 做成試用版，在 gittu 上看過 Pack、Fill、Even、Gaps + Fill、Gaps 五種之後：「Fill」

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

## History 與 Working tree 合成一頁（2026-11-22）

user 照原話：「1. 我想要把 working tree 和 history 的 tab 合併，把 working tree 的 changes panel 放到 history panel 1 的上方, 當 focus 在 changes 的時候，右邊 panel 2 會變成 diff, 然後 panel 3 換成 panel 2 cursor 的那個檔案的 history, panel 4 隱藏
2. title header statusbar 和 history/working tree 的 tab 就可以移除了，我們在 panel 2 的上面建立一個只可以觀看的 panel, 裡面放 branch 和 repo
---
這樣就可以把兩頁合成一頁，並且釋放上方高度」

## 左邊 [1] 與 [2] 的高度 1:2（2026-11-22）

user 照原話：「左側的 panel 1 和 panel 2 高度比例為 1:2」

## 拿掉 STATUS，狀態寫在 [1] 的位置列（2026-11-22）

user 照原話：「然後我發現這樣 status 那個 panel 可以整個移除」
問 merge 的狀態與 Pushing… 要放哪之後：「放到 changes 那個 All changes 的那一個 row, 當有狀況的時候，就不需要顯示 All changes, All changes 只是個佔位文字」

## 多列項目的 cursor（2026-11-22）

user 照原話：「我覺得那的多行顯示的 cursor ui 很糟糕」
問要怎麼處理之後：「先回報，這需要在討論，我也還沒想怎樣作比較好」

## 用 < > 調左右的寬度，照寬度記下來（2026-11-23）

user 照原話：「我想討論一個功能，就是讓使用者可以使用'>' '<' 來設定左側和右側的寬度，當 focus 在右邊的 panel '<' 就是放大右側寬度 '>' 就是縮小左側寬度，當 focus 在左側 panel, '<' 就是縮小左側寬物, '>' 就是放大左側寬度, app 會紀錄當前是窄寬還是長寬視窗，依照當前寬度紀錄這個大小設定，之後使用者開啟 app, 如果是符合紀錄的設定，就直接採用設定好的左右配比, 這樣右邊 panel 的 zoom 就不需要兩段式了，直接滿版 toggle 就好了」
問「依照寬度記」要每種寬度各記一筆（A）還是只分窄、寬兩種（B）之後：「A」

## [1] CHANGES 的 Enter 不再 stage（2026-11-23）

user 照原話：「changes panel 的熱鍵轉換一下，enter stage 移除，換一個可以 toggle stage 的熱鍵」

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
