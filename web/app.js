const $ = (id) => document.getElementById(id);
const fields = ['peak', 'bw', 'batch', 'dimD', 'dimF', 'aBytes', 'wBytes', 'oBytes'];
let calculationNumber = 0;
function val(id) {
  return Number($(id).value)
}
function sci(v,d=3) {
  if(!Number.isFinite(v))return '—';
  return new Intl.NumberFormat('en-US', {
    notation:'compact',maximumFgitractionDigits:d
  }
  ).format(v)
}
function clearStats() {
  calculationNumber++;
  ['intensity','ridge','throughput'].forEach(x=>$(x).textContent='—');
  $('regime').textContent='Waiting';
  $('regimeHint').textContent='Add values to begin';
  $('formulas').innerHTML='<div class="empty">Enter positive values for hardware and matrix dimensions to see the worked calculation.</div>';
  resetChart()
}
async function preset(k) {
  try {
    const response=await fetch('/api/hardware/'+encodeURIComponent(k));
    if(!response.ok)throw new Error('Unable to load hardware profile');
    const p=await response.json();
    $('hardware').value=k;
    $('peak').value=p.peak;
    $('bw').value=p.bw;
    document.querySelectorAll('.preset').forEach(b=>b.classList.toggle('active',b.dataset.hw===k));
    calculate()
  }
  catch(error) {
    $('error').textContent=error.message
  }
}
async function calculate() {
  const [peak,bw,B,D,F,a,w,o]=fields.map(val),inputs= {
    peak,bw,B,D,F,a,w,o
  }
  ,requestNumber=++calculationNumber;
  if(Object.values(inputs).some(x=>!(x>0))) {
    $('error').textContent=Object.values(inputs).some(x=>x<0)?'Values must be positive.':'Fill in all fields to calculate; hardware presets fill peak and bandwidth.';
    clearStats();
    return
  }
  $('error').textContent='';
  try {
    const response=await fetch('/api/calculate', {
      method:'POST',headers: {
        'Content-Type':'application/json'
      }
      ,body:JSON.stringify(inputs)
    }
    );
    if(!response.ok)throw new Error((await response.text()).trim());
    const result=await response.json();
    if(requestNumber!==calculationNumber)return;
    $('intensity').textContent=sci(result.intensity);
    $('ridge').textContent=sci(result.ridge);
    $('throughput').textContent=sci(result.rate)+' FLOP/s';
    $('regime').textContent=result.regime;
    $('regime').style.color=result.regime==='Compute-bound'?'var(--accent)':'var(--bluegray)';
    $('regimeHint').textContent=result.regime==='Compute-bound'?'At the compute ceiling':'Limited by data movement';
    $('formulas').innerHTML=result.formula_html;
    $('chart').outerHTML=result.plot_svg
  }
  catch(error) {
    if(requestNumber===calculationNumber) {
      $('error').textContent='Calculation failed: '+error.message;
      clearStats()
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
    clearStats()
  }
}
);
$('calculate').addEventListener('click',calculate);
$('clear').addEventListener('click',()=> {
  fields.forEach(id=>$(id).value='');
  $('hardware').value='custom';
  document.querySelectorAll('.preset').forEach(b=>b.classList.remove('active'));
  $('error').textContent='';
  clearStats()
}
);
fields.forEach(id=>$(id).addEventListener('input',()=> {
  if(fields.every(k=>val(k)>0))calculate()
}
));
resetChart();
