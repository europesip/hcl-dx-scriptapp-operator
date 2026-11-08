// The clock of sample 05. The time zone comes from a substitution (data-zone of the script tag).
(function () {
  var me = document.currentScript, zone = (me && me.getAttribute("data-zone")) || undefined;
  function start() {
    var el = document.querySelector(".dxop-clock [data-time]");
    if (!el) { return setTimeout(start, 200); }
    function tick() { el.textContent = new Date().toLocaleTimeString("en-GB", { timeZone: zone }); }
    tick(); setInterval(tick, 1000);
  }
  start();
})();
