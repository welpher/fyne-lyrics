# 行内卡拉OK（逐字高亮）支持 — 设计文档

日期：2026-09-10
状态：已实现

## 背景与目标

fyne-lyrics 是一个 Fyne 歌词显示 widget，支持 synced / unsynced 两种模式，
原本由外部通过 `NextLine()` / `SetCurrentLine(n)` 以"行"为粒度驱动。

需要兼容带行内标签的逐字歌词格式，例如：

```lrc
[00:00.00]<00:00.00>床<00:00.40>前<00:00.80>明<00:01.20>月<00:01.60>光<00:02.00> <00:02.40>(Moonlight)<00:03.00>...
```

即每一行文本中内嵌 `<MM:SS.CC>` 标签，指示其后字符（字/词）的开始时间（百分之一秒精度）。
目标：在活动行内，随播放时间逐字点亮（卡拉OK效果）。

### 与下游（supersonic）的关系

supersonic 已经剥离了行首 `[...]` 标签（行起始时间存入它自己的 `LyricLine.Start`），
传给 fyne-lyrics 的是 `lyric.Lines[i].Text`。经确认：

- lrclib 解析路径：`Text` 中**保留** `<...>` 行内标签。
- subsonic 结构化歌词路径：`Text` 为纯文本，`Start` 单独存放，**无**行内标签。

因此本 widget 不能假设每行必带行内标签，解析必须支持两种情况，且对缺失场景优雅回退。

## 设计原则

- **完全向后兼容**：现有 API 一个不动，普通歌词行为不变。
- **模式由"是否调用 SetPlayTime"决定**：
  - 从未调用 → 保持现有 `NextLine`/`SetCurrentLine` 行为原样。
  - 开始调用 → 进入"时间驱动模式"，widget 随每次 `SetPlayTime` 自主推进行切换、字符点亮与滚动。
- **性能**：push 提供时间基线 + 短距预测定时器修正字边界精度；仅在状态变化时重绘。

## 新增公开 API

### 1. 播放时间输入（push 模式）

```go
// 外部在每个播放时间更新时调用。curTime 为当前播放时间（秒），
// seeked 表示本次更新是否源于 seek。
// 从未调用 SetPlayTime 时，widget 保持现有 NextLine / SetCurrentLine 行为；
// 一旦调用，进入时间驱动模式。
// 必须在 UI 线程调用：它会修改 widget 状态并启停动画。
func (l *LyricsViewer) SetPlayTime(curTime float64, seeked bool)
```

采用 push 而非 pull（函数句柄）：

- supersonic 已有 `OnPlayTimeUpdate` 轮询循环（普通 250ms、waveform 时 100ms），
  设置一次回调直接转发即可，外部零额外负担。
- 调用粒度由供给方轮询频率决定（250ms 量级），非"逐字高频"，无性能顾虑。
- widget 只维护一个**短距预测定时器**用于字边界精度修正，无长驻 goroutine、无自建调度循环。

### 2. 可选的行起始时间（增强稳健性）

```go
// 每行起始时间（秒），与 SetLyrics 的 lines 严格一一对应。
// nil 时，行的活动窗口由行内首字时间推导（[本行首字, 下一行首字)）。
// 主要用于无行内标签的歌词（如 subsonic 结构化路径）也能正确切换行。
LineStarts []float64
```

该字段可选，不设不影响依赖行内标签的主流场景。

## 行内标签解析

新增解析逻辑，将 `SetLyrics` 收到的每行文本拆解为词片段：

```go
type word struct {
    text  string
    start float64 // 秒；-1 表示无时间（未检测到标签的文本）
}
```

- 检测 `<MM:SS.CC>`：
  - 标签后跟随的文本（到下一个 `<` 为止）作为该片段的 `text`。
  - `start = min*60 + ss + cc/100`。
  - 标签前未配对的文本归入 `start = -1` 的无时间片段，按顺序拼接显示。
- 无任何标签 → 整行作为单一无时间片段，走回退逻辑（显示不受影响）。
- 解析函数独立成纯函数（`parseWords`），便于单测。

## 渲染（时间驱动模式）

### 关键约束：字体回退与基线

在真实 GL 驱动（尤其 macOS + supersonic 的 fyne fork）下，**每个独立文本对象会各自做
字体回退（font fallback）**。把一行拆成多个文本对象（每字/每词一个）时，同一个字形
可能解析到不同的回退字体，**基线不同 → 字与字之间高低错位**。

注意：`RenderedTextSize` 会报告各字 baseline 相同（如全部 19.23），但实际绘制不同——
**测量值不可靠**。因此"按测量基线手动对齐多个文本对象"无法解决该问题。

**唯一可靠的方案是一次 shaping**：go-text 在排版**单个字符串**时，会把所有回退 run
对齐到同一基线；拆成多个字符串则各管各的。

### 实现：整行 base + 同一字符串裁剪叠加

`lyricLine` 不再为每个词建独立文本段，而是：

- `baseText`：整行文本（所有词拼接）的一个 `canvas.Text`，`InactiveLyricColor` 色
- `activeText`：**与 baseText 完全相同的整行字符串**的另一个 `canvas.Text`，`ActiveLyricColor` 色
- `activeText` 外套一个裁剪控件 `lyricClip`，裁剪宽度 = 已唱前缀的**像素宽度**

原理：

- 两层字符串完全相同 → shaping 逐字一致 → 字形像素级重合，**无重影**
- 裁剪只露出已唱前缀 → 实现逐字高亮
- 裁剪宽度用 `canvas.Text` 测量前缀（与渲染同一条测量路径），避免边界切进下一个字

**裁剪的跨版本兼容**（关键）：

| Fyne 版本 | 裁剪触发机制 |
|---|---|
| Fyne 2.5.x | 对象实现 `fyne.Scrollable` |
| supersonic 的 fyne fork | renderer 实现 `IsClip()` |

`lyricClip` **同时实现两者**，故两个版本都能正确裁剪。
synced 模式下 `scroll.Direction = ScrollNone`（本就不能滚动），因此用 `Scrollable`
标记不会影响滚动；unsynced 模式无行内标签、不产生多词行，也就不会用到裁剪。

### 行窗口推导

`LineStarts` 为 nil 时，行的活动时间窗口为
`[本行第一个带时间片段.start, 下一行第一个带时间片段.start)`；
无法推导时，全行使用 InactiveLyricColor 静态显示。

### 活动行滚动

沿用现有 `offsetForLine` 逻辑：

- 普通行切换：动画滚动到精确目标偏移（`offsetForLine(currentLine)`）并居中
- `seeked=true`（seek）：**瞬时定位**到精确偏移、不带动画
- 逐字点亮只更新叠加层的裁剪宽度，不触发整行滚动

## 内部推进：push + 预测定时器

时间驱动模式下，`SetPlayTime`（约 250ms 一次）提供粗粒度时间基线，内部用一个
**短距预测定时器**补足字边界精度，让字在其 `start` 时刻精确点亮，而不是等到下一次 push。

预测定时器以 **`fyne.Animation`** 实现（而非 `time.AfterFunc`）：Fyne 2.5.3 无 `fyne.Do`，
而 `fyne.Animation` 的回调在 UI 线程运行、widget 不可见时自动停止，无需自建
goroutine 或线程调度。每次 `SetPlayTime` 会取消并重排该动画。

**每次 `SetPlayTime(curTime, seeked)`：**

1. 记录 `lastCurTime = curTime`、`lastPushAt = now`（墙上时钟）；取消已挂起的预测动画。
2. 以 `curTime` 定位当前行 + `activeWordIdx`（`seeked=true` 强制全量；否则增量）。
3. 找下一个字/行边界 `T_next`（> `curTime` 的最近一个）。
4. 若 `T_next - curTime <= predictiveHorizon`（1s），挂起 `fyne.Animation(T_next - curTime)`；
   否则不挂（等下次 push）。

**预测动画完成时（回调在 UI 线程）：**

1. `elapsed = now - lastPushAt`。
2. **Stall 检测**：若 `elapsed > stallThreshold`（600ms，约 2.5× 250ms poll）→
   判定暂停/停止，取消后续调度并返回（下次 `SetPlayTime` 恢复）。
3. 否则：点亮当前行内 `start <= T_next` 的字（推进 `activeWordIdx`），必要时切换行/滚动。
4. 找下一个边界，按步骤 4 规则继续挂起。

**暂停安全性**：supersonic 暂停时 `OnPlayTimeUpdate` 停推（已确认 `OnPaused → stopPollTimePos`）。
纯 push 天然冻结；预测定时器靠 stall 检测保持暂停安全——暂停期间最多误推进约 `stallThreshold`
（约 1 字），恢复后首推立即按真实 `curTime` 纠正。

**边界**：

- `predictiveHorizon` 限制单次预测跨度，避免间奏段挂远期定时器做长程墙钟假设。
- 倍速播放（rate≠1）下预测有偏差，由下次 push 纠正，可接受。
- `SetLyrics` / widget 销毁 / `seeked=true` 时取消挂起的预测动画。

### 渲染开销

每次定位变化时重建整行 base/active 两个 `canvas.Text` 的文本与颜色。
每行只有 2 个文本对象，代价可忽略；不做逐段增量着色。

## 高亮粒度（v1 范围）

- **逐字变色**（默认、v1 实现）：字到达其 `start` 时，裁剪宽度推进到该字之后，
  对应字切为 Active 色。
- **平滑渐变**（可选增强，暂不实现）：字与字之间颜色随进度插值渐变，
  需要每帧重绘，成本高。YAGNI，留作后续。

## 兼容性与集成

- 旧 API（`SetLyrics`/`SetCurrentLine`/`NextLine`/`OnLyricTapped`）全部保留。
- 从未调用 `SetPlayTime` 时，行为与现在完全一致。
- 普通（无行内标签）歌词在时间驱动模式下照常显示，只是没有逐字效果；
  `LineStarts` 提供时行切换仍正确。
- supersonic 集成：

```go
pm.OnPlayTimeUpdate(func(cur, _ float64, seeked bool) {
    fyne.Do(func() { lyricsViewer.SetPlayTime(cur, seeked) })
})
```

其 `UpdatePlayPos`/`OnSeeked` 中驱动 `NextLine`/`SetCurrentLine` 的逻辑停用，
改为直接转发 `SetPlayTime`；`SetLyrics` 时额外传入 `LineStarts`。

## 错误与边界处理

- 歌词行为空文本 → 不建词片段，显示空行（不变）。
- 标签格式非法（如 `<xx:xx>` 无法解析）→ 跳过该标签，文本按字面保留。
- `LineStarts` 长度与 lines 不一致 → 以较短者为准，剩余行走行内首字推导或静态显示。
- `SetPlayTime` 的 `curTime` 为负值或明显异常 → 视为 0，按最前位置处理。
- `advanceTo` 在 `currentLine == 0`（尚未进入任何行）时安全跳过，不越界。
- 重复调用 `SetLyrics` → 全量重置（当前行回首行、清零 `activeWordIdx`、重置 `timeDriven`），
  并取消挂起的预测动画。
- 一次播放时间更新期间多次调用 `SetPlayTime` → 幂等，仅在定位变化时重绘。

## 测试策略

- **解析**（`wordparse_test.go`）：标准 `<MM:SS.CC>`、无标签行、混合文本、非法标签、中文/空格/多字节。
- **纯逻辑**（`karaoke_test.go`）：`locateActiveLine` 行定位、`countActiveWords` 已唱字数、`nextBoundary` 下一边界。
- **视图**（`lyricsviewer_test.go`）：`SetLyrics` 解析 `lineWords`/`lineStarts`；外部 `LineStarts` 优先。
- **渲染**（`lyricline_test.go`）：多词行的 base 与 active 层字符串完全一致、位置重合、存在裁剪叠加层。
- **行切换**（`line_transition_test.go`）：跨行边界时新行的 `activeWordIdx` 用目标行词语计算，
  不出现"整行先全亮"。
- **兼容**：从未调用 `SetPlayTime` 时现有单测/行为不回归。

## 文件清单（实现）

- `wordparse.go` / `wordparse_test.go`：`<MM:SS.CC>` 解析纯函数。
- `karaoke.go` / `karaoke_test.go`：时间驱动纯逻辑（行定位、已唱字数、下一边界）。
- `lyricline.go` / `lyricline_test.go`：单次 shaping 渲染 + 裁剪叠加层。
- `lyricsviewer.go` / `lyricsviewer_test.go`：`SetPlayTime`、`LineStarts`、预测定时器、行切换。
- `line_transition_test.go`：行切换逐字进度回归。
- `demo/karaoke/main.go`：内联歌词的 demo（模拟播放 + 点按 seek）。
