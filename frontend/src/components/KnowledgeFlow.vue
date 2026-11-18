<template>
  <div class="knowledge-flow" aria-hidden="true">
    <div class="knowledge-flow__header">
      <span>KNOWLEDGE / CONNECTIONS</span>
      <span class="knowledge-flow__status"><i /> KNOWLEDGE FLOW</span>
    </div>
    <svg class="knowledge-flow__diagram" viewBox="0 0 620 330" role="presentation">
      <defs>
        <pattern id="knowledge-grid" width="28" height="28" patternUnits="userSpaceOnUse">
          <path d="M 28 0 L 0 0 0 28" fill="none" stroke="#ffffff" stroke-opacity=".07" stroke-width="1" />
        </pattern>
      </defs>
      <rect width="620" height="330" fill="url(#knowledge-grid)" />
      <path class="knowledge-flow__rail" d="M174 66 C220 66 219 132 266 132 M174 165 H266 M174 264 C220 264 219 198 266 198 M388 165 H455" />
      <path class="knowledge-flow__signal knowledge-flow__signal--one" d="M174 66 C220 66 219 132 266 132" />
      <path class="knowledge-flow__signal knowledge-flow__signal--two" d="M174 165 H266" />
      <path class="knowledge-flow__signal knowledge-flow__signal--three" d="M174 264 C220 264 219 198 266 198" />
      <path class="knowledge-flow__signal knowledge-flow__signal--four" d="M388 165 H455" />

      <circle class="knowledge-flow__packet" r="4">
        <animateMotion dur="3.6s" repeatCount="indefinite" path="M174 66 C220 66 219 132 266 132" />
      </circle>
      <circle class="knowledge-flow__packet" r="4">
        <animateMotion dur="3.6s" begin="-1.2s" repeatCount="indefinite" path="M174 165 H266" />
      </circle>
      <circle class="knowledge-flow__packet" r="4">
        <animateMotion dur="3.6s" begin="-2.4s" repeatCount="indefinite" path="M174 264 C220 264 219 198 266 198" />
      </circle>
      <circle class="knowledge-flow__packet knowledge-flow__packet--answer" r="4">
        <animateMotion dur="2.2s" begin="-1.6s" repeatCount="indefinite" path="M388 165 H455" />
      </circle>

      <g class="knowledge-flow__node knowledge-flow__node--one" transform="translate(20 36)">
        <rect width="154" height="60" rx="5" />
        <path d="M17 18h15m-15 7h22m-22 7h18" />
        <text x="51" y="36">DOCUMENT</text>
      </g>
      <g class="knowledge-flow__node knowledge-flow__node--two" transform="translate(20 135)">
        <rect width="154" height="60" rx="5" />
        <path d="M17 19h22m-22 8h16m-16 8h20" />
        <text x="51" y="36">Q&amp;A</text>
      </g>
      <g class="knowledge-flow__node knowledge-flow__node--three" transform="translate(20 234)">
        <rect width="154" height="60" rx="5" />
        <path d="M17 19h22m-22 8h19m-19 8h16" />
        <text x="51" y="36">VIDEO</text>
      </g>

      <g class="knowledge-flow__core" transform="translate(266 106)">
        <rect width="122" height="118" rx="5" />
        <path d="M29 28h64M29 45h64M29 62h64" />
        <circle cx="42" cy="28" r="5" />
        <circle cx="76" cy="45" r="5" />
        <circle cx="55" cy="62" r="5" />
        <text x="61" y="98" text-anchor="middle">SENSEDEEP</text>
      </g>

      <g class="knowledge-flow__result" transform="translate(455 126)">
        <rect width="145" height="78" rx="5" />
        <path d="M19 27h90M19 40h68M19 53h80" />
        <text x="19" y="67">ANSWER</text>
      </g>
    </svg>
    <div class="knowledge-flow__footer">
      <span>01 / SOURCES</span>
      <span>02 / INDEX</span>
      <span>03 / ANSWERS</span>
    </div>
  </div>
</template>

<style scoped>
.knowledge-flow {
  width: 100%;
  max-width: 620px;
  border-top: 1px solid rgba(222, 240, 236, .25);
  border-bottom: 1px solid rgba(222, 240, 236, .25);
  color: #bed8d3;
}
.knowledge-flow__header,
.knowledge-flow__footer {
  display: flex;
  justify-content: space-between;
  align-items: center;
  gap: 12px;
  padding: 12px 1px;
  font: 600 10px/1.3 ui-monospace, SFMono-Regular, Consolas, monospace;
}
.knowledge-flow__footer { color: #86b5ac; }
.knowledge-flow__status { display: inline-flex; align-items: center; gap: 8px; color: #e9b27c; }
.knowledge-flow__status i { width: 6px; height: 6px; background: currentColor; border-radius: 50%; animation: statusPulse 2.4s ease-in-out infinite; }
.knowledge-flow__diagram { display: block; width: 100%; height: auto; }
.knowledge-flow__rail { fill: none; stroke: #70aca3; stroke-opacity: .35; stroke-width: 1.5; }
.knowledge-flow__signal { fill: none; stroke: #f2b77c; stroke-width: 2; stroke-dasharray: 4 13; stroke-linecap: round; animation: signalTravel 2.2s linear infinite; }
.knowledge-flow__signal--two { animation-delay: -.5s; }
.knowledge-flow__signal--three { animation-delay: -1s; }
.knowledge-flow__signal--four { animation-delay: -.75s; stroke: #bde8db; }
.knowledge-flow__packet { fill: #ffd099; stroke: #fff0d5; stroke-width: 1.5; filter: drop-shadow(0 0 5px #f9b979); }
.knowledge-flow__packet--answer { fill: #d2fff3; stroke: #fff; filter: drop-shadow(0 0 5px #bde8db); }
.knowledge-flow__node rect { fill: #24514e; stroke: #61968d; stroke-width: 1; }
.knowledge-flow__node path { fill: none; stroke: #9fc7bf; stroke-width: 2; stroke-linecap: round; }
.knowledge-flow__node text,
.knowledge-flow__result text { fill: #e6f3f0; font: 600 12px ui-monospace, SFMono-Regular, Consolas, monospace; }
.knowledge-flow__node { animation: nodeFocus 6s ease-in-out infinite; }
.knowledge-flow__node--two { animation-delay: -2s; }
.knowledge-flow__node--three { animation-delay: -4s; }
.knowledge-flow__core rect { fill: #e4f3ee; stroke: #ffffff; }
.knowledge-flow__core path { fill: none; stroke: #32746e; stroke-width: 2; stroke-linecap: round; }
.knowledge-flow__core circle { fill: #edaf72; }
.knowledge-flow__core text { fill: #164540; font: 700 11px ui-monospace, SFMono-Regular, Consolas, monospace; }
.knowledge-flow__result rect { fill: #36615b; stroke: #a8d1c6; }
.knowledge-flow__result path { fill: none; stroke: #a8d1c6; stroke-width: 2; stroke-linecap: round; }
.knowledge-flow__result path { stroke-dasharray: 90; animation: answerReveal 3.6s ease-in-out infinite; }
@keyframes signalTravel { to { stroke-dashoffset: -34; } }
@keyframes answerReveal { 0%, 25% { stroke-dashoffset: 90; } 50%, 85% { stroke-dashoffset: 0; } 100% { stroke-dashoffset: 90; } }
@keyframes nodeFocus { 0%, 15%, 100% { opacity: .7; } 30%, 60% { opacity: 1; } }
@keyframes statusPulse { 50% { opacity: .35; } }
@media (prefers-reduced-motion: reduce) {
  .knowledge-flow__signal,
  .knowledge-flow__node,
  .knowledge-flow__status i,
  .knowledge-flow__result path { animation: none; }
  .knowledge-flow__packet { display: none; }
}
</style>
