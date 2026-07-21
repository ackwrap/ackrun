## Q：部分用户在更新至最新版本的 Chrome，开启TUN模式 Chrome 浏览器无法正常上网，而其他浏览器（如 Edge）却能正常使用，如图片加载异常。

A: 修改 Chrome Flags 配置（推荐首选）,在 Chrome 地址栏中输入以下路径并回车，将该项设置为 Disabled，然后重启浏览器：
```
chrome://flags/#local-network-access-check
```