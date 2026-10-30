import React from 'react';
import type { PeerRecord } from '../../types/node';

interface MeshTopologyMapProps {
  peers: PeerRecord[];
  localPeerId?: string;
  cacheLimit?: number;
}

export const MeshTopologyMap: React.FC<MeshTopologyMapProps> = ({
  peers,
  localPeerId,
  cacheLimit = 64,
}) => {
  const width = 360;
  const height = 240;
  const cx = width / 2;
  const cy = height / 2;
  const r1 = 64;
  const r2 = 104;

  const connectedPeers = peers.filter((p) => p.connected);
  const knownPeers = peers.filter((p) => !p.connected);

  const posMap = new Map<string, { x: number; y: number }>();

  const connectedNodes = connectedPeers.map((peer, idx) => {
    const angle = (2 * Math.PI * idx) / Math.max(1, connectedPeers.length) - Math.PI / 2;
    const x = cx + r1 * Math.cos(angle);
    const y = cy + r1 * Math.sin(angle);
    posMap.set(peer.peer_id, { x, y });
    return { peer, x, y };
  });

  const knownNodes = knownPeers.map((peer, idx) => {
    const angle = (2 * Math.PI * idx) / Math.max(1, knownPeers.length) - Math.PI / 4;
    const x = cx + r2 * Math.cos(angle);
    const y = cy + r2 * Math.sin(angle);
    posMap.set(peer.peer_id, { x, y });
    return { peer, x, y };
  });

  return (
    <div
      style={{
        display: 'flex',
        flexDirection: 'column',
        gap: '8px',
        backgroundColor: 'var(--bg-card)',
        border: '1px solid rgba(255, 255, 255, 0.05)',
        borderRadius: 'var(--radius-sm)',
        padding: '10px',
      }}
    >
      <svg
        viewBox={`0 0 ${width} ${height}`}
        style={{
          width: '100%',
          height: '210px',
          display: 'block',
          userSelect: 'none',
        }}
      >
        {/* Concentric Horizon Rings */}
        <circle
          cx={cx}
          cy={cy}
          r={r1}
          fill="none"
          stroke="rgba(255, 255, 255, 0.05)"
          strokeWidth={1}
          strokeDasharray="3 3"
        />
        <circle
          cx={cx}
          cy={cy}
          r={r2}
          fill="none"
          stroke="rgba(255, 255, 255, 0.03)"
          strokeWidth={1}
          strokeDasharray="2 4"
        />

        {/* 2-Hop Referral Links (from 1-hop neighbor or center to 2-hop peer) */}
        {knownNodes.map(({ peer, x, y }) => {
          const viaPos = peer.discovered_via ? posMap.get(peer.discovered_via) : undefined;
          const startX = viaPos ? viaPos.x : cx;
          const startY = viaPos ? viaPos.y : cy;
          return (
            <line
              key={`edge-2hop-${peer.peer_id}`}
              x1={startX}
              y1={startY}
              x2={x}
              y2={y}
              stroke="rgba(161, 161, 170, 0.25)"
              strokeWidth={1}
              strokeDasharray="2 3"
            />
          );
        })}

        {/* 1-Hop Active Swarm Links */}
        {connectedNodes.map(({ peer, x, y }, idx) => {
          const isQuarantined = Boolean(peer.is_quarantined);
          const isRelayed = peer.connection_type === 'relayed';
          const strokeColor = isQuarantined
            ? 'rgba(244, 63, 94, 0.5)'
            : isRelayed
            ? 'rgba(56, 189, 248, 0.55)'
            : 'rgba(16, 185, 129, 0.6)';
          const pathId = `swarm-link-${idx}`;

          return (
            <g key={`edge-1hop-${peer.peer_id}`}>
              <path
                id={pathId}
                d={`M ${x} ${y} L ${cx} ${cy}`}
                fill="none"
                stroke={strokeColor}
                strokeWidth={1.5}
                strokeDasharray={isRelayed || isQuarantined ? '4 3' : undefined}
              />
              {!isQuarantined && (peer.blocks_verified ?? 0) > 0 && (
                <circle r={2.5} fill={isRelayed ? '#38bdf8' : '#10b981'}>
                  <animateMotion dur="1.8s" repeatCount="indefinite">
                    <mpath href={`#${pathId}`} />
                  </animateMotion>
                </circle>
              )}
            </g>
          );
        })}

        {/* 2-Hop Known Nodes */}
        {knownNodes.map(({ peer, x, y }) => (
          <g key={`node-2hop-${peer.peer_id}`}>
            <title>
              {`2-Hop Known Peer: ${peer.peer_id}${
                peer.discovered_via ? `\nDiscovered via: ${peer.discovered_via}` : ''
              }`}
            </title>
            <circle
              cx={x}
              cy={y}
              r={5}
              fill="#27272a"
              stroke="rgba(161, 161, 170, 0.6)"
              strokeWidth={1.2}
            />
            <text
              x={x}
              y={y + 13}
              textAnchor="middle"
              fill="#71717a"
              fontSize="8"
              fontFamily="var(--font-mono)"
            >
              {peer.peer_id.slice(-4)}
            </text>
          </g>
        ))}

        {/* 1-Hop Connected Swarm Nodes */}
        {connectedNodes.map(({ peer, x, y }) => {
          const isQuarantined = Boolean(peer.is_quarantined);
          const isRelayed = peer.connection_type === 'relayed';
          const fill = isQuarantined ? '#f43f5e' : isRelayed ? '#38bdf8' : '#10b981';
          const latency = peer.avg_latency_ms;

          return (
            <g key={`node-1hop-${peer.peer_id}`}>
              <title>
                {`Peer: ${peer.peer_id}\nType: ${peer.connection_type || 'direct'}\nScore: ${(
                  peer.reputation_score ?? 100
                ).toFixed(0)} pts${latency ? `\nLatency: ~${latency.toFixed(0)}ms` : ''}`}
              </title>
              <circle
                cx={x}
                cy={y}
                r={8}
                fill="var(--bg-surface)"
                stroke={fill}
                strokeWidth={2.2}
              />
              <circle cx={x} cy={y} r={3.2} fill={fill} />
              <text
                x={x}
                y={y - 11}
                textAnchor="middle"
                fill="#e4e4e7"
                fontSize="8.5"
                fontFamily="var(--font-mono)"
                fontWeight={600}
              >
                {peer.peer_id.slice(-5)}
              </text>
              {latency && latency > 0 ? (
                <text
                  x={x}
                  y={y + 17}
                  textAnchor="middle"
                  fill="#a1a1aa"
                  fontSize="7.5"
                  fontFamily="var(--font-mono)"
                >
                  ~{latency.toFixed(0)}ms
                </text>
              ) : null}
            </g>
          );
        })}

        {/* Local Center Node */}
        <g>
          <title>{localPeerId ? `Local Node: ${localPeerId}` : 'Local Node'}</title>
          <circle
            cx={cx}
            cy={cy}
            r={13}
            fill="var(--bg-surface)"
            stroke="var(--accent-primary)"
            strokeWidth={2.2}
          />
          <circle cx={cx} cy={cy} r={4.5} fill="var(--accent-primary)" />
          <text
            x={cx}
            y={cy + 23}
            textAnchor="middle"
            fill="var(--text-primary)"
            fontSize="8.5"
            fontFamily="var(--font-mono)"
            fontWeight={700}
          >
            LOCAL
          </text>
        </g>
      </svg>

      {/* Legend & Bounded Memory Footer */}
      <div
        style={{
          display: 'flex',
          alignItems: 'center',
          justifyContent: 'space-between',
          flexWrap: 'wrap',
          gap: '8px',
          paddingTop: '6px',
          borderTop: '1px solid rgba(255, 255, 255, 0.05)',
          fontSize: '0.68rem',
          color: 'var(--text-muted)',
        }}
      >
        <div style={{ display: 'flex', alignItems: 'center', gap: '10px', flexWrap: 'wrap' }}>
          <span style={{ display: 'flex', alignItems: 'center', gap: '4px' }}>
            <span style={{ width: 6, height: 6, borderRadius: '50%', backgroundColor: '#10b981' }} />
            Direct P2P ({connectedPeers.filter((p) => !p.is_quarantined && p.connection_type !== 'relayed').length})
          </span>
          <span style={{ display: 'flex', alignItems: 'center', gap: '4px' }}>
            <span style={{ width: 6, height: 6, borderRadius: '50%', backgroundColor: '#38bdf8' }} />
            Relay ({connectedPeers.filter((p) => !p.is_quarantined && p.connection_type === 'relayed').length})
          </span>
          <span style={{ display: 'flex', alignItems: 'center', gap: '4px' }}>
            <span style={{ width: 6, height: 6, borderRadius: '50%', backgroundColor: '#71717a' }} />
            2-Hop PEX ({knownPeers.length})
          </span>
        </div>
        <span style={{ fontFamily: 'var(--font-mono)', fontSize: '0.66rem' }}>
          Cache: {peers.length}/{cacheLimit} · TTL 30m
        </span>
      </div>
    </div>
  );
};
