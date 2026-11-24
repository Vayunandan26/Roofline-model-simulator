const $ = (id) => document.getElementById(id);
const fields = ['peak', 'bw', 'batch', 'dimD', 'dimF', 'aBytes', 'wBytes', 'oBytes'];
let calculationNumber = 0;
let rooflineCalculate;
const profiles = {
  v5e: {name:'TPU v5e · BF16', peak:1.97e14, bw:8.2e11},
  h100: {name:'NVIDIA H100 · BF16', peak:9.89e14, bw:3.35e12},
  v6e: {name:'TPU v6e · BF16', peak:9.1e14, bw:1.6e12}
};
const wasmReady = (async () => {
  if (typeof Go === 'undefined') return false;
  if (!('WebAssembly' in window)) throw new Error('This browser does not support WebAssembly');
  const go = new Go();
  const wasmURL = new URL('roofline.wasm', document.baseURI);
  const response = await fetch(wasmURL);
  if (!response.ok) throw new Error(`Unable to load WebAssembly module (${response.status})`);
  const bytes = await response.arrayBuffer();
  const {instance} = await WebAssembly.instantiate(bytes, go.importObject);
  go.run(instance).catch(error => { $('error').textContent = `WebAssembly stopped: ${error.message}`; });
  for (let i=0; i<200 && typeof window.rooflineCalculate !== 'function'; i++) {
    await new Promise(resolve => setTimeout(resolve, 10));
  }
  if (typeof window.rooflineCalculate !== 'function') throw new Error('WebAssembly module did not initialize');
  rooflineCalculate = window.rooflineCalculate;
  return true;
})();
function val(id) { return Number($(id).value) }
function clearResults() {
  calculationNumber++;
  $('formulas').innerHTML='<div class="empty">Enter positive values for hardware and matrix dimensions to see the worked calculation.</div>';
  resetChart()
}
function preset(k) {
  const p=profiles[k];
  if(!p) return;
  $('hardware').value=k;
  $('peak').value=p.peak;
  $('bw').value=p.bw;
  document.querySelectorAll('.preset').forEach(b=>b.classList.toggle('active',b.dataset.hw===k));
  calculate()
}
async function calculate() {
  const [peak,bw,B,D,F,a,w,o]=fields.map(val), inputs={peak,bw,B,D,F,a,w,o}, requestNumber=++calculationNumber;
  if(Object.values(inputs).some(x=>!(x>0))) {
    $('error').textContent=Object.values(inputs).some(x=>x<0)?'Values must be positive.':'Fill in all fields to calculate; hardware presets fill peak and bandwidth.';
    clearResults();
    return
  }
  $('error').textContent='';
  try {
    const useWasm = await wasmReady;
    if(requestNumber!==calculationNumber)return;
    let result;
    if (useWasm) {
      result=JSON.parse(rooflineCalculate(JSON.stringify(inputs)));
    } else {
      const response=await fetch('/api/calculate', {
        method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify(inputs)
      });
      if(!response.ok)throw new Error((await response.text()).trim());
      result=await response.json();
    }
    if(result.error)throw new Error(result.error);
    if(requestNumber!==calculationNumber)return;
    $('formulas').innerHTML=result.formula_html;
    $('chart').outerHTML=result.plot_svg
  }
  catch(error) {
    if(requestNumber===calculationNumber) {
      $('error').textContent='Calculation failed: '+error.message;
      clearResults()
    }
  }
}
const emptyPlot=`<rect width="800" height="380" fill="white"/>
  <g stroke="#e6e9e5">
<line x1="78" y1="24" x2="78" y2="322"/>
  <line x1="194.2" y1="24" x2="194.2" y2="322"/>
  <line x1="310.3" y1="24" x2="310.3" y2="322"/>
  <line x1="426.5" y1="24" x2="426.5" y2="322"/>
  <line x1="542.7" y1="24" x2="542.7" y2="322"/>
  <line x1="658.8" y1="24" x2="658.8" y2="322"/>
  <line x1="775" y1="24" x2="775" y2="322"/>
<line x1="78" y1="24" x2="775" y2="24"/>
  <line x1="78" y1="73.7" x2="775" y2="73.7"/>
  <line x1="78" y1="123.3" x2="775" y2="123.3"/>
  <line x1="78" y1="173" x2="775" y2="173"/>
  <line x1="78" y1="222.7" x2="775" y2="222.7"/>
  <line x1="78" y1="272.3" x2="775" y2="272.3"/>
  <line x1="78" y1="322" x2="775" y2="322"/>
  </g>
<g fill="#78827c" font-size="10">
  <text x="78" y="342" text-anchor="middle">0.01</text>
  <text x="194.2" y="342" text-anchor="middle">0.1</text>
  <text x="310.3" y="342" text-anchor="middle">1</text>
  <text x="426.5" y="342" text-anchor="middle">10</text>
  <text x="542.7" y="342" text-anchor="middle">100</text>
  <text x="658.8" y="342" text-anchor="middle">1000</text>
  <text x="775" y="342" text-anchor="middle">10000</text>
  <text x="68" y="28" text-anchor="end">10000T</text>
  <text x="68" y="78" text-anchor="end">1000T</text>
  <text x="68" y="127" text-anchor="end">100T</text>
  <text x="68" y="177" text-anchor="end">10T</text>
  <text x="68" y="227" text-anchor="end">1T</text>
  <text x="68" y="276" text-anchor="end">100B</text>
  <text x="68" y="326" text-anchor="end">10B</text>
  </g>
  <text x="426.5" y="173" text-anchor="middle" fill="#8b9590" font-size="12">Enter hardware values to draw a roofline</text>
  <rect x="78" y="24" width="697" height="298" fill="none" stroke="#c7cec8"/>
  <text x="426.5" y="368" text-anchor="middle" fill="#59645e" font-size="12">Arithmetic intensity (FLOPs / byte)</text>
  <text transform="translate(18 173) rotate(-90)" text-anchor="middle" fill="#59645e" font-size="12">Peak achievable FLOP/s</text>`;
function resetChart() {
  $('chart').innerHTML=emptyPlot
}
document.querySelectorAll('.tab').forEach(t=>t.addEventListener('click',()=> {
  document.querySelectorAll('.tab').forEach(x=>x.classList.remove('active'));
  document.querySelectorAll('.view').forEach(x=>x.classList.remove('active'));
  t.classList.add('active');
  $(t.dataset.view).classList.add('active')
}
));
document.querySelectorAll('.preset').forEach(b=>b.addEventListener('click',()=>preset(b.dataset.hw)));
$('hardware').addEventListener('change',e=> {
  if(e.target.value!=='custom')preset(e.target.value);
  else {
    document.querySelectorAll('.preset').forEach(b=>b.classList.remove('active'));
    clearResults()
  }
}
);
$('calculate').addEventListener('click',calculate);
$('clear').addEventListener('click',()=> {
  fields.forEach(id=>$(id).value='');
  $('hardware').value='custom';
  document.querySelectorAll('.preset').forEach(b=>b.classList.remove('active'));
  $('error').textContent='';
  clearResults()
}
);
fields.forEach(id=>$(id).addEventListener('input',()=> {
  if(fields.every(k=>val(k)>0))calculate()
}
));
resetChart();
