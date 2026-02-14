import { useEffect, useRef, useCallback } from 'react';
import { useTerminalSocket } from '../../api/websocket.js';

export default function TerminalView() {
  var containerRef = useRef(null);
  var termRef = useRef(null);
  var { connect, disconnect, sendData } = useTerminalSocket(termRef);

  useEffect(function () {
    if (!containerRef.current || !window.Terminal) return;

    var term = new window.Terminal({
      theme: {
        background: '#0A0B0E',
        foreground: '#F4F5F7',
        cursor: '#E8A838',
      },
      fontFamily: "'JetBrains Mono', monospace",
      fontSize: 12,
    });

    termRef.current = term;
    term.open(containerRef.current);

    if (window.FitAddon) {
      var fitAddon = new window.FitAddon.FitAddon();
      term.loadAddon(fitAddon);
      fitAddon.fit();

      var resizeObserver = new ResizeObserver(function () {
        try { fitAddon.fit(); } catch (_) {}
      });
      resizeObserver.observe(containerRef.current);
    }

    term.onData(function (data) {
      sendData(data);
    });

    connect();

    return function () {
      disconnect();
      term.dispose();
    };
  }, [connect, disconnect, sendData]);

  return (
    <div ref={containerRef} style={{ width: '100%', height: '100%', background: '#0A0B0E' }} />
  );
}
