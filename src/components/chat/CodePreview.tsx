import { useMemo, useState } from "react";
import { transform } from "sucrase";
import { Eye, EyeOff } from "lucide-react";
import { Button } from "@/components/ui/button";

const csp = "default-src 'none'; script-src 'unsafe-inline'; style-src 'unsafe-inline'; img-src data:; font-src data:; connect-src 'none'; base-uri 'none'; form-action 'none'; child-src 'none'";

function escapeScript(value: string) {
  return value.replace(/<\/script/gi, "<\\/script");
}

function reactShim() {
  return `
const React={createElement(type,props,...children){return {type,props:{...(props||{}),children}}}};
function render(node,parent){if(node==null||node===false)return;if(typeof node==='string'||typeof node==='number'){parent.append(document.createTextNode(String(node)));return}if(typeof node.type==='function'){render(node.type(node.props),parent);return}const el=document.createElement(node.type);for(const [key,value] of Object.entries(node.props||{})){if(key==='children'||value==null||value===false)continue;if(key==='className'){el.className=value;continue}if(key==='style'&&typeof value==='object'){Object.assign(el.style,value);continue}if(key.startsWith('on')&&typeof value==='function'){el.addEventListener(key.slice(2).toLowerCase(),value);continue}if(value===true){el.setAttribute(key,'');continue}el.setAttribute(key,String(value))}for(const child of node.props.children||[])render(child,el);parent.append(el)}
const ReactDOM={createRoot(root){return {render(element){root.replaceChildren();render(element,root)}}}};
`;
}

function jsxDocument(source: string) {
  const compiled = transform(source, { transforms: ["jsx"], jsxPragma: "React.createElement" }).code;
  return `<!doctype html><meta http-equiv="Content-Security-Policy" content="${csp}"><body><div id="root"></div><script>${reactShim()}${escapeScript(compiled)}\nReactDOM.createRoot(document.getElementById('root')).render(React.createElement(App));<\/script>`;
}

function htmlDocument(source: string) {
  const safe = escapeScript(source);
  return `<!doctype html><meta http-equiv="Content-Security-Policy" content="${csp}">${safe}`;
}

export function CodePreview({ language, code }: { language: string; code: string }) {
  const [open, setOpen] = useState(false);
  const document = useMemo(() => {
    try {
      return language === "jsx" ? jsxDocument(code) : htmlDocument(code);
    } catch (error) {
      return htmlDocument(`<pre>Preview error: ${String(error)}</pre>`);
    }
  }, [language, code]);

  return <div className="border-t bg-background/50 px-3 py-2">
    <Button type="button" size="sm" variant="ghost" className="gap-1.5" onClick={() => setOpen((value) => !value)} aria-expanded={open}>
      {open ? <EyeOff className="size-3.5" /> : <Eye className="size-3.5" />}{open ? "Tutup preview" : "Preview"}
    </Button>
    {open && <iframe title={`Preview ${language}`} sandbox="allow-scripts" srcDoc={document} className="mt-2 h-80 w-full rounded-md border bg-white" />}
  </div>;
}
