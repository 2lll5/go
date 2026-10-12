# Repository Agent Instructions

## Google Drive Build 交付規範（所有 Agent 必須遵守）
- 每個 Repo 各有專屬 Google Drive `AI Project/go/` 資料夾，名稱必須與 GitHub Repository 相同（AI Project ID `1Yhfro6Ss5TSCXCVIZIHEu5h7V1gL8b27`）。不要把 Build 散放在 AI Project 頂層。
- 最新**完整、經測試**的 Build（ZIP／EXE、SHA-256、所有分卷、還原腳本和必要說明）**直接放專案根目錄**，不每次都加版本資料夾。各平台最新版可並存。上傳後核對檔案名稱、大小、parent 及 checksum；新版缺 ZIP、缺分卷或只有 SHA 時不取代舊版。
- **只有重大功能／架構／美術／相容性差異才另建 `vX.Y.Z-說明/` 里程碑子資料夾**。普通舊版移入 `歷史封存/`，保留原檔、不刪除。
- 新開發同步 GitHub；完整／昂貴 Actions Build 以 `workflow_dispatch` 或正式 `release: published` 控制，不要每次推送 main 都觸發。回報專案 Drive 連結、版本、檔案與驗證結果。只有實際 Build、上傳成功才能稱完成。
**English for Codex:** Publish only the latest verified complete build(s) directly to `AI Project/go/`; archive routine older builds, and use version subfolders solely for important milestones. Never promote incomplete uploads.
