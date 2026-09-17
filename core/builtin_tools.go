package core

// WorkspaceRoot 統一工作空間根目錄與虛擬根歸并映射已上收至 workspace.go
// （宿主與插件進程同源；插件經 SDK 二次封裝使用，見 core.MapWorkspacePath /
// core.ResolveWorkspacePath）。
