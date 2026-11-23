
// —— 以下到 COMPONENT_END 为止，CSR 与 SSR 用的是同一份字符串 ——
function ItemList(props) {
  return React.createElement('ul', { className: 'items' },
    props.items.map(function (it) {
      return React.createElement('li', { key: it.id },
        React.createElement('h2', null, it.title),
        React.createElement('p', null, it.summary),
        React.createElement('span', null, it.author));
    }));
}
function App(props) {
  return React.createElement('main', null,
    React.createElement('h1', null, '条目列表'),
    React.createElement('p', { className: 'meta' }, '共 ' + props.items.length + ' 条'),
    React.createElement(ItemList, { items: props.items }));
}
var ProbeEffect = function () {
  // 水合完成的信号：effect 只在客户端跑，服务端渲染不出它
  React.useEffect(function () { performance.mark('hydrated'); }, []);
  return null;
};
// —— COMPONENT_END ——

var root = ReactDOM.hydrateRoot(document.getElementById('root'),
  React.createElement(React.Fragment, null,
    React.createElement(App, { items: window.__ITEMS__.items }),
    React.createElement(ProbeEffect)));
