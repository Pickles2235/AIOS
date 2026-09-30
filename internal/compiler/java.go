package compiler

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/AdamNi-7080/AIOS/internal/model"
)

func java(ctx context.Context, repo model.Repository, files []model.File, runtime model.CompilerRuntime, dataDir string) (Result, error) {
	home := runtime.JavaHome
	javac, javaBin := "javac", "java"
	if home != "" {
		javac = filepath.Join(home, "bin", "javac")
		javaBin = filepath.Join(home, "bin", "java")
	}
	if _, err := exec.LookPath(javac); err != nil {
		return Result{}, fmt.Errorf("jdk compiler unavailable: %w", err)
	}
	d, err := cache(dataDir, "java")
	if err != nil {
		return Result{}, err
	}
	source, err := writeHelper(d, "OutAboutJavac.java", javaHelper)
	if err != nil {
		return Result{}, err
	}
	classes := filepath.Join(d, "classes")
	sum := sha256.Sum256([]byte(javaHelper))
	stamp := hex.EncodeToString(sum[:])
	stored, stampErr := os.ReadFile(filepath.Join(classes, "OutAboutJavac.sha256"))
	if _, err = os.Stat(filepath.Join(classes, "OutAboutJavac.class")); err != nil || stampErr != nil || string(stored) != stamp {
		if err = os.MkdirAll(classes, 0700); err != nil {
			return Result{}, err
		}
		c := exec.CommandContext(ctx, javac, "-proc:none", "-d", classes, source)
		if b, e := c.CombinedOutput(); e != nil {
			return Result{}, fmt.Errorf("compile javac helper: %w: %s", e, bounded(string(b)))
		}
		if err = os.WriteFile(filepath.Join(classes, "OutAboutJavac.sha256"), []byte(stamp), 0600); err != nil {
			return Result{}, err
		}
	}
	paths := make([]string, 0, len(files))
	for _, f := range files {
		paths = append(paths, absolute(repo.Root, f.Path))
	}
	sort.Strings(paths)
	args := append([]string{"-cp", classes, "OutAboutJavac"}, paths...)
	cmd := exec.CommandContext(ctx, javaBin, args...)
	cmd.Env = append(os.Environ(), "AIOS_ROOT="+repo.Root)
	records, err := run(ctx, cmd)
	if err != nil {
		return Result{}, err
	}
	r := toResult("java", records)
	for i := range r.Symbols {
		r.Symbols[i].RepoID = repo.ID
	}
	for i := range r.Edges {
		r.Edges[i].RepoID = repo.ID
	}
	return r, nil
}

// The helper intentionally uses only javac public APIs. It parses and
// analyzes supplied source files, never calls generate(), and does not run
// annotation processors. Output is JSONL consumed only by the parent process.
const javaHelper = `
import java.io.*; import java.nio.file.*; import java.util.*; import javax.tools.*; import com.sun.source.tree.*; import com.sun.source.util.*; import javax.lang.model.element.*;
public class OutAboutJavac {
 static String q(String s){return "\""+s.replace("\\","\\\\").replace("\"","\\\"").replace("\n","\\n").replace("\r","")+"\"";}
 static void out(String type,String path,String name,String kind,String id,String src,String tgt,String sid,String tid,String pred,long a,long b,CompilationUnitTree u,SourcePositions p){LineMap l=u.getLineMap(); long sl=l.getLineNumber(a), sc=l.getColumnNumber(a), el=l.getLineNumber(b), ec=l.getColumnNumber(b); System.out.println("{\"Type\":"+q(type)+",\"Path\":"+q(path)+",\"Name\":"+q(name)+",\"Kind\":"+q(kind)+",\"Identity\":"+q(id)+",\"Source\":"+q(src)+",\"Target\":"+q(tgt)+",\"SourceIdentity\":"+q(sid)+",\"TargetIdentity\":"+q(tid)+",\"Predicate\":"+q(pred)+",\"StartByte\":"+a+",\"EndByte\":"+b+",\"StartLine\":"+sl+",\"StartColumn\":"+sc+",\"EndLine\":"+el+",\"EndColumn\":"+ec+"}"); }
 static String id(Element e){ if(e==null)return ""; Element owner=e.getEnclosingElement(); String o=owner==null?"":owner.toString(); return "java:"+e.getKind()+":"+o+"#"+e.toString(); }
 static String label(Element e){return e==null?"":e.getSimpleName().toString();}
 static String rel(Path root,CompilationUnitTree u){try{return root.relativize(Paths.get(u.getSourceFile().toUri())).toString().replace('\\','/');}catch(Exception e){return u.getSourceFile().getName();}}
 public static void main(String[] args)throws Exception { JavaCompiler c=ToolProvider.getSystemJavaCompiler(); if(c==null)throw new IllegalStateException("JDK compiler unavailable"); StandardJavaFileManager fm=c.getStandardFileManager(null,null,null); List<File> fs=new ArrayList<>();for(String a:args)fs.add(new File(a)); Iterable<? extends JavaFileObject> in=fm.getJavaFileObjectsFromFiles(fs); JavacTask task=(JavacTask)c.getTask(null,fm,null,List.of("-proc:none","-implicit:none"),null,in); Iterable<? extends CompilationUnitTree> units=task.parse(); try{task.analyze();}catch(Exception ignored){} Trees trees=Trees.instance(task); SourcePositions pos=trees.getSourcePositions(); Path root=Paths.get(System.getenv("AIOS_ROOT")).toAbsolutePath(); for(CompilationUnitTree u:units){ String path=rel(root,u); new TreePathScanner<Void,Void>() { String enclosing=""; String enclosingID=""; void sym(Tree n,Element e,String kind){long a=pos.getStartPosition(u,n),b=pos.getEndPosition(u,n);if(a>=0&&b>=a)out("symbol",path,label(e),kind,id(e),"","","","","",a,b,u,pos);}
  @Override public Void visitClass(ClassTree n,Void v){Element e=trees.getElement(getCurrentPath());sym(n,e,"type");String old=enclosing,oi=enclosingID;enclosing=label(e);enclosingID=id(e);Tree ext=n.getExtendsClause();if(ext!=null){Element t=trees.getElement(new TreePath(getCurrentPath(),ext));edge(ext,t,"EXTENDS");}for(Tree x:n.getImplementsClause()){Element t=trees.getElement(new TreePath(getCurrentPath(),x));edge(x,t,"IMPLEMENTS");}Void z=super.visitClass(n,v);enclosing=old;enclosingID=oi;return z;}
  @Override public Void visitMethod(MethodTree n,Void v){Element e=trees.getElement(getCurrentPath());String old=enclosing,oi=enclosingID;enclosing=label(e);enclosingID=id(e);sym(n,e,e!=null&&e.getKind()==ElementKind.CONSTRUCTOR?"constructor":"method");Void z=super.visitMethod(n,v);enclosing=old;enclosingID=oi;return z;}
  @Override public Void visitVariable(VariableTree n,Void v){Element e=trees.getElement(getCurrentPath());sym(n,e,"field");return super.visitVariable(n,v);}
  @Override public Void visitImport(ImportTree n,Void v){Element e=trees.getElement(getCurrentPath());edge(n.getQualifiedIdentifier(),e,"IMPORTS");return super.visitImport(n,v);}
  void edge(Tree n,Element target,String predicate){if(target==null)return;long a=pos.getStartPosition(u,n),b=pos.getEndPosition(u,n);if(a>=0&&b>=a)out("edge",path,"","","",enclosing,label(target),enclosingID,id(target),predicate,a,b,u,pos);}
  @Override public Void visitMethodInvocation(MethodInvocationTree n,Void v){Element e=trees.getElement(getCurrentPath());edge(n.getMethodSelect(),e,"CALLS");return super.visitMethodInvocation(n,v);}
  @Override public Void visitIdentifier(IdentifierTree n,Void v){Element e=trees.getElement(getCurrentPath()); if(e!=null && !id(e).equals(enclosingID))edge(n,e,"REFERENCES");return super.visitIdentifier(n,v);}
 }.scan(u,null); } fm.close(); }
}`

var _ = strings.Builder{}
