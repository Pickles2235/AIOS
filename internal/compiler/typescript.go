package compiler

import (
	"context"
	"fmt"
	"os"
	"os/exec"

	"github.com/AdamNi-7080/AIOS/internal/model"
)

func typescript(ctx context.Context, repo model.Repository, config string, runtime model.CompilerRuntime, dataDir string) (Result, error) {
	if runtime.Node == "" {
		return Result{}, fmt.Errorf("node runtime is not configured")
	}
	d, err := cache(dataDir, "typescript")
	if err != nil {
		return Result{}, err
	}
	helper, err := writeHelper(d, "aios-typescript.js", typescriptHelper)
	if err != nil {
		return Result{}, err
	}
	module := runtime.TypeScriptModule
	if module == "" { // Node resolves the repository-local package from tsconfig.
		module = "local"
	}
	command := exec.CommandContext(ctx, runtime.Node, helper, module, absolute(repo.Root, config))
	command.Env = append(os.Environ(), "AIOS_ROOT="+repo.Root)
	records, err := run(ctx, command)
	if err != nil {
		return Result{}, err
	}
	r := toResult("typescript", records)
	for i := range r.Symbols {
		r.Symbols[i].RepoID = repo.ID
	}
	for i := range r.Edges {
		r.Edges[i].RepoID = repo.ID
	}
	return r, nil
}

const typescriptHelper = `
const fs=require('fs'),path=require('path');
const modulePath=process.argv[2], config=process.argv[3], configDir=path.dirname(config), root=path.resolve(process.env.AIOS_ROOT);
let ts; try { ts=modulePath==='local'?require(require.resolve('typescript',{paths:[configDir]})):require(modulePath); } catch(e){ console.error('typescript unavailable: '+e.message); process.exit(2); }
function emit(x){ process.stdout.write(JSON.stringify(x)+'\n'); }
function span(sf,n){ const a=n.getStart(sf,false), b=n.getEnd(), p=sf.getLineAndCharacterOfPosition(a), q=sf.getLineAndCharacterOfPosition(b); return {StartByte:a,EndByte:b,StartLine:p.line+1,StartColumn:p.character+1,EndLine:q.line+1,EndColumn:q.character+1}; }
function rel(f){return path.relative(root,f).replaceAll(path.sep,'/');}
function ident(sym){ if(!sym)return ''; const d=sym.declarations&&sym.declarations[0]; return 'typescript:'+String(sym.flags)+':'+(d?rel(d.getSourceFile().fileName)+':'+d.pos:'external')+':'+sym.getName(); }
function name(n){ return n.name&&ts.isIdentifier(n.name)?n.name.text : n.name&&n.name.getText?n.name.getText():''; }
const read=ts.readConfigFile(config,ts.sys.readFile); if(read.error){ emit({Type:'diagnostic',Path:rel(config),Message:ts.flattenDiagnosticMessageText(read.error.messageText,' ')}); process.exit(0); }
const parsed=ts.parseJsonConfigFileContent(read.config,ts.sys,configDir,{noEmit:true,incremental:false},config); const program=ts.createProgram({rootNames:parsed.fileNames,options:{...parsed.options,noEmit:true,incremental:false,tsBuildInfoFile:undefined},projectReferences:parsed.projectReferences}); const checker=program.getTypeChecker();
for(const d of ts.getPreEmitDiagnostics(program).slice(0,100)) emit({Type:'diagnostic',Path:d.file?rel(d.file.fileName):rel(config),Message:ts.flattenDiagnosticMessageText(d.messageText,' '),...(d.file&&d.start!==undefined?span(d.file,{getStart:()=>d.start,getEnd:()=>d.start+d.length}):{})});
for(const sf of program.getSourceFiles()){ if(sf.isDeclarationFile||sf.fileName.includes('/node_modules/'))continue; const file=rel(sf.fileName); let owner='',ownerId=''; function edge(n,target,predicate){if(!target)return;emit({Type:'edge',Path:file,Source:owner,Target:target.getName(),SourceIdentity:ownerId,TargetIdentity:ident(target),Predicate:predicate,...span(sf,n)});} function visit(n){ let s=checker.getSymbolAtLocation(n.name||n); if(ts.isClassDeclaration(n)||ts.isInterfaceDeclaration(n)||ts.isFunctionDeclaration(n)||ts.isEnumDeclaration(n)||ts.isTypeAliasDeclaration(n)||ts.isMethodDeclaration(n)||ts.isVariableDeclaration(n)||ts.isPropertyDeclaration(n)||ts.isJsxOpeningElement(n)||ts.isJsxSelfClosingElement(n)){ if(s){let k=ts.SyntaxKind[n.kind].replace('Declaration','').toLowerCase(); emit({Type:'symbol',Path:file,Name:name(n)||s.getName(),Kind:k,Identity:ident(s),...span(sf,n)}); let old=owner,oldId=ownerId; owner=s.getName();ownerId=ident(s); ts.forEachChild(n,visit);owner=old;ownerId=oldId;return;} }
 if(ts.isImportDeclaration(n)||ts.isExportDeclaration(n)){ let mod=n.moduleSpecifier&&checker.getSymbolAtLocation(n.moduleSpecifier); edge(n,mod,ts.isImportDeclaration(n)?'IMPORTS':'EXPORTS'); }
 if(ts.isImportSpecifier(n)&&n.propertyName){ edge(n,checker.getSymbolAtLocation(n.propertyName),'ALIASES'); }
 if(ts.isCallExpression(n)){ let s=checker.getResolvedSignature(n); let d=s&&s.declaration&&checker.getSymbolAtLocation(s.declaration.name); edge(n.expression,d,'CALLS'); if(n.expression.kind===ts.SyntaxKind.ImportKeyword) emit({Type:'diagnostic',Path:file,Message:'dynamic import retained as unresolved coverage gap',...span(sf,n)}); }
 if(ts.isHeritageClause(n)){ for(const t of n.types){let s=checker.getSymbolAtLocation(t.expression);edge(t,s,n.token===ts.SyntaxKind.ExtendsKeyword?'EXTENDS':'IMPLEMENTS');} }
 if(ts.isIdentifier(n)){let s=checker.getSymbolAtLocation(n);if(s&&ident(s)!==ownerId)edge(n,s,'REFERENCES');} ts.forEachChild(n,visit); }
 ts.forEachChild(sf,visit); }
`
