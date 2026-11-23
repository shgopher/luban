
// 子应用 a：真实项目里这里会 import 一堆公共包（组件库/工具库/i18n）
(function () {
  var total = 0;
  for (var i = 0; i < 260; i++) total += SharedLib.fns['f' + i](i);
  var el = document.getElementById('a');
  el.innerHTML = '<button class="btn">a 的按钮</button><span class="sum">' + total + '</span>';
  window.__mounted = (window.__mounted || []).concat('a');
  if (window.__mounted.length === 2) performance.mark('content-ready');
})();
