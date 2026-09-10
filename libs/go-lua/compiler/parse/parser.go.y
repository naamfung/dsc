%{
package parse

import (
  "strconv"
  "strings"
  "github.com/wippyai/go-lua/compiler/ast"
)

func parseNumber(s string) (interface{}, error) {
  if strings.HasPrefix(s, "0x") || strings.HasPrefix(s, "0X") {
    i, err := strconv.ParseInt(s, 0, 64)
    return i, err
  }
  f, err := strconv.ParseFloat(s, 64)
  if err != nil {
    return f, err
  }
  if f == float64(int64(f)) && !strings.Contains(s, ".") && !strings.ContainsAny(s, "eE") {
    return int64(f), nil
  }
  return f, nil
}

func setLastPosFromExprs(node ast.PositionHolder, exprs []ast.Expr, fallback ast.PositionHolder) {
  if node == nil {
    return
  }
  if len(exprs) > 0 {
    node.CopyLastPos(exprs[len(exprs)-1])
    return
  }
  if fallback != nil {
    node.CopyLastPos(fallback)
  }
}
%}
%type<stmts> chunk
%type<stmts> chunk1
%type<stmts> block
%type<stmt>  stat
%type<stmts> elseifs
%type<stmt>  laststat
%type<funcname> funcname
%type<funcname> funcname1
%type<exprlist> varlist
%type<expr> var
%type<namelist> namelist
%type<exprlist> exprlist
%type<expr> expr
%type<expr> string
%type<expr> prefixexp
%type<expr> functioncall
%type<expr> afunctioncall
%type<exprlist> args
%type<expr> function
%type<funcexpr> funcbody
%type<parlist> parlist
%type<expr> tableconstructor
%type<fieldlist> fieldlist
%type<field> field
%type<fieldsep> fieldsep
%type<fieldname> fieldname
%type<fieldname> typefieldname

%type<typeexpr> typeexpr
%type<typeexpr> simpletypeexpr
%type<typeexpr> primarytypeexpr
%type<typeexprlist> typeexprlist
%type<typeexprlist> typeexprlist2
%type<typeexprlist> returntypeannot
%type<typeparams> typeparams
%type<typeparams> typeparamlist
%type<typeparam> typeparam
%type<recordfields> typefieldlist
%type<recordfield> typefield
%type<typednames> typednamelist
%type<typedname> typedname
%type<ifacemethods> interfacebody
%type<ifacemethod> interfacemethod
%type<typereflist> interfaceextends
%type<funcparam> funcparam
%type<funcparams> funcparamlist
%type<annotation> annotation
%type<annotations> annotations

%union {
  token  ast.Token

  stmts    []ast.Stmt
  stmt     ast.Stmt

  funcname *ast.FuncName
  funcexpr *ast.FunctionExpr

  exprlist []ast.Expr
  expr   ast.Expr

  fieldlist []*ast.Field
  field     *ast.Field
  fieldsep  string
  fieldname string

  namelist []nameWithPos
  parlist  *ast.ParList

  typeexpr     ast.TypeExpr
  typeexprlist []ast.TypeExpr
  typeparam    ast.TypeParamExpr
  typeparams   []ast.TypeParamExpr
  recordfield  ast.RecordFieldExpr
  recordfields []ast.RecordFieldExpr
  typedname    typedNameEntry
  typednames   []typedNameEntry
  ifacemethod  ast.InterfaceMethodExpr
  ifacemethods []ast.InterfaceMethodExpr
  typereflist  []*ast.TypeRefExpr
  funcparam    ast.FunctionParamExpr
  funcparams   []ast.FunctionParamExpr
  annotation   ast.AnnotationExpr
  annotations  []ast.AnnotationExpr
}

/* Reserved words */
%token<token> TAnd TBreak TDo TElse TElseIf TEnd TFalse TFor TFunction TIf TIn TLocal TNil TNot TOr TReturn TRepeat TThen TTrue TUntil TWhile TGoto

/* Type system keywords */
%token<token> TType TInterface TReadonly TAs TAsserts TIs TTypeof TKeyof TExtends TFun

/* Literals */
%token<token> TEqeq TNeq TLte TGte T2Comma T3Comma T2Colon TLabel TIdent TNumber TString '{' '}' '(' ')'

/* Lua 5.3 operators */
%token<token> TShl TShr TIdiv

/* Type annotation operators */
%token<token> TArrow TQuestion TBang TQuestionColon

/* Operators - Lua 5.3 precedence */
%left TOr
%left TAnd
%left '>' '<' TGte TLte TEqeq TNeq
%left '|'
%left '~'
%left '&'
%left TShl TShr
%right T2Comma
%left '+' '-'
%left '*' '/' '%' TIdiv
%right UNARY /* not # -(unary) ~(bnot) */
%right '^'
%nonassoc T2Colon /* :: cast - nonassoc to prefer reduce over shift for labels */
%left TAs TBang /* type cast (as) and non-nil assertion */

/* Known shift/reduce conflicts (14 total, all resolved correctly by shift):
   - 3 Lua inherent: prefixexp '(' ambiguity (call vs grouping) - cannot be eliminated
   - 5 type optional: simpletypeexpr TQuestion binds tighter than union/intersection
   - 2 function return: (params) -> typeexpr followed by |/& binds to return type
   - 2 type decl: type Name pattern - shift to continue parsing type name
   - 2 generic: TIdent '<' in type context - shift for generic args
*/

%%

chunk: 
        chunk1 {
            $$ = $1
            if l, ok := yylex.(*Lexer); ok {
                l.Stmts = $$
            }
        } |
        chunk1 laststat {
            $$ = append($1, $2)
            if l, ok := yylex.(*Lexer); ok {
                l.Stmts = $$
            }
        } | 
        chunk1 laststat ';' {
            $$ = append($1, $2)
            if l, ok := yylex.(*Lexer); ok {
                l.Stmts = $$
            }
        }

chunk1: 
        {
            $$ = []ast.Stmt{}
        } |
        chunk1 stat {
            $$ = append($1, $2)
        } | 
        chunk1 ';' {
            $$ = $1
        }

block: 
        chunk {
            $$ = $1
        }

stat:
        varlist '=' exprlist {
            $$ = &ast.AssignStmt{Lhs: $1, Rhs: $3}
            $$.CopyPos($1[0])
        } |
        /* 'stat = functioncal' causes a reduce/reduce conflict */
        prefixexp {
            if _, ok := $1.(*ast.FuncCallExpr); !ok {
               yylex.(*Lexer).Error("parse error")
            } else {
              $$ = &ast.FuncCallStmt{Expr: $1}
              $$.CopyPos($1)
            }
        } |
        TDo block TEnd {
            $$ = &ast.DoBlockStmt{Stmts: $2}
            $$.SetPosFromToken($1.Pos)
            $$.SetLastPosFromToken($3.Pos)
        } |
        TWhile expr TDo block TEnd {
            $$ = &ast.WhileStmt{Condition: $2, Stmts: $4}
            $$.SetPosFromToken($1.Pos)
            $$.SetLastPosFromToken($5.Pos)
        } |
        TRepeat block TUntil expr {
            $$ = &ast.RepeatStmt{Condition: $4, Stmts: $2}
            $$.SetPosFromToken($1.Pos)
            $$.SetLastLine($4.Line())
        } |
        TIf expr TThen block elseifs TEnd {
            $$ = &ast.IfStmt{Condition: $2, Then: $4}
            cur := $$
            for _, elseif := range $5 {
                cur.(*ast.IfStmt).Else = []ast.Stmt{elseif}
                cur = elseif
            }
            $$.SetPosFromToken($1.Pos)
            $$.SetLastPosFromToken($6.Pos)
        } |
        TIf expr TThen block elseifs TElse block TEnd {
            $$ = &ast.IfStmt{Condition: $2, Then: $4}
            cur := $$
            for _, elseif := range $5 {
                cur.(*ast.IfStmt).Else = []ast.Stmt{elseif}
                cur = elseif
            }
            cur.(*ast.IfStmt).Else = $7
            $$.SetPosFromToken($1.Pos)
            $$.SetLastPosFromToken($8.Pos)
        } |
        TFor TIdent '=' expr ',' expr TDo block TEnd {
            $$ = &ast.NumberForStmt{Name: $2.Str, NamePosition: $2.Pos, Init: $4, Limit: $6, Stmts: $8}
            $$.SetPosFromToken($1.Pos)
            $$.SetLastPosFromToken($9.Pos)
        } |
        TFor TIdent '=' expr ',' expr ',' expr TDo block TEnd {
            $$ = &ast.NumberForStmt{Name: $2.Str, NamePosition: $2.Pos, Init: $4, Limit: $6, Step:$8, Stmts: $10}
            $$.SetPosFromToken($1.Pos)
            $$.SetLastPosFromToken($11.Pos)
        } |
        TFor namelist TIn exprlist TDo block TEnd {
            names, positions := splitNameList($2)
            $$ = &ast.GenericForStmt{Names: names, NamePositions: positions, Exprs:$4, Stmts: $6}
            $$.SetPosFromToken($1.Pos)
            $$.SetLastPosFromToken($7.Pos)
        } |
        TFunction funcname funcbody {
            $$ = &ast.FuncDefStmt{Name: $2, Func: $3}
            $$.SetPosFromToken($1.Pos)
            $$.CopyLastPos($3)
        } |
        TLocal TFunction TIdent funcbody {
            $$ = &ast.LocalAssignStmt{Names:[]string{$3.Str}, NamePositions: []ast.Position{$3.Pos}, Exprs: []ast.Expr{$4}}
            $$.SetPosFromToken($1.Pos)
            $$.CopyLastPos($4)
        } |
        TLocal typednamelist '=' exprlist {
            names, positions, types := splitTypedNames($2)
            $$ = &ast.LocalAssignStmt{Names: names, NamePositions: positions, Types: types, Exprs: $4}
            $$.SetPosFromToken($1.Pos)
        } |
        TLocal typednamelist {
            names, positions, types := splitTypedNames($2)
            $$ = &ast.LocalAssignStmt{Names: names, NamePositions: positions, Types: types, Exprs: []ast.Expr{}}
            $$.SetPosFromToken($1.Pos)
        } |
        TLabel {
            $$ = &ast.LabelStmt{Name: $1.Str}
            $$.SetPosFromToken($1.Pos)
        } |
        TGoto TIdent {
            $$ = &ast.GotoStmt{Label: $2.Str}
            $$.SetPosFromToken($1.Pos)
        } |
        TIdent TIdent '=' typeexpr {
            if $1.Str != "type" {
                yylex.(*Lexer).Error("unexpected identifier")
            }
            $$ = &ast.TypeDefStmt{Name: $2.Str, Type: $4}
            $$.SetPosFromToken($1.Pos)
        } |
        TIdent TIdent typeparams '=' typeexpr {
            if $1.Str != "type" {
                yylex.(*Lexer).Error("unexpected identifier")
            }
            $$ = &ast.TypeDefStmt{Name: $2.Str, TypeParams: $3, Type: $5}
            $$.SetPosFromToken($1.Pos)
        } |
        TInterface TIdent interfaceextends interfacebody TEnd {
            $$ = &ast.InterfaceDefStmt{Name: $2.Str, Extends: $3, Methods: $4}
            $$.SetPosFromToken($1.Pos)
            $$.SetLastPosFromToken($5.Pos)
        }

elseifs: 
        {
            $$ = []ast.Stmt{}
        } | 
        elseifs TElseIf expr TThen block {
            $$ = append($1, &ast.IfStmt{Condition: $3, Then: $5})
            $$[len($$)-1].SetPosFromToken($2.Pos)
        }

laststat:
        TReturn {
            $$ = &ast.ReturnStmt{Exprs:nil}
            $$.SetPosFromToken($1.Pos)
        } |
        TReturn exprlist {
            $$ = &ast.ReturnStmt{Exprs:$2}
            $$.SetPosFromToken($1.Pos)
        } |
        TBreak  {
            $$ = &ast.BreakStmt{}
            $$.SetPosFromToken($1.Pos)
        }

funcname:
        funcname1 {
            $$ = $1
        } |
        funcname1 ':' TIdent {
            $$ = &ast.FuncName{Func:nil, Receiver:$1.Func, Method: $3.Str}
        } |
        funcname1 ':' TType {
            $$ = &ast.FuncName{Func:nil, Receiver:$1.Func, Method: "type"}
        } |
        funcname1 ':' TInterface {
            $$ = &ast.FuncName{Func:nil, Receiver:$1.Func, Method: "interface"}
        } |
        funcname1 ':' TReadonly {
            $$ = &ast.FuncName{Func:nil, Receiver:$1.Func, Method: "readonly"}
        } |
        funcname1 ':' TAs {
            $$ = &ast.FuncName{Func:nil, Receiver:$1.Func, Method: "as"}
        } |
        funcname1 ':' TAsserts {
            $$ = &ast.FuncName{Func:nil, Receiver:$1.Func, Method: "asserts"}
        } |
        funcname1 ':' TIs {
            $$ = &ast.FuncName{Func:nil, Receiver:$1.Func, Method: "is"}
        }

funcname1:
        TIdent {
            $$ = &ast.FuncName{Func: &ast.IdentExpr{Value:$1.Str}}
            $$.Func.SetPosFromToken($1.Pos)
        } | 
        funcname1 '.' TIdent {
            key:= &ast.StringExpr{Value:$3.Str}
            key.SetPosFromToken($3.Pos)
            key.SetLastPosFromToken($3.Pos)
            fn := &ast.AttrGetExpr{Object: $1.Func, Key: key}
            fn.CopyPos($1.Func)
            fn.SetLastPosFromToken($3.Pos)
            $$ = &ast.FuncName{Func: fn}
        }

varlist:
        var {
            $$ = []ast.Expr{$1}
        } | 
        varlist ',' var {
            $$ = append($1, $3)
        }

var:
        TIdent {
            $$ = &ast.IdentExpr{Value:$1.Str}
            $$.SetPosFromToken($1.Pos)
        } |
        prefixexp '[' expr ']' {
            $$ = &ast.AttrGetExpr{Object: $1, Key: $3}
            $$.CopyPos($1)
        } | 
        prefixexp '.' TIdent {
            key := &ast.StringExpr{Value:$3.Str}
            key.SetPosFromToken($3.Pos)
            key.SetLastPosFromToken($3.Pos)
            $$ = &ast.AttrGetExpr{Object: $1, Key: key}
            $$.CopyPos($1)
            $$.SetLastPosFromToken($3.Pos)
        } |
        prefixexp '.' TType {
            key := &ast.StringExpr{Value:"type"}
            key.SetPosFromToken($3.Pos)
            key.SetLastPosFromToken($3.Pos)
            $$ = &ast.AttrGetExpr{Object: $1, Key: key}
            $$.CopyPos($1)
            $$.SetLastPosFromToken($3.Pos)
        } |
        prefixexp '.' TInterface {
            key := &ast.StringExpr{Value:"interface"}
            key.SetPosFromToken($3.Pos)
            key.SetLastPosFromToken($3.Pos)
            $$ = &ast.AttrGetExpr{Object: $1, Key: key}
            $$.CopyPos($1)
            $$.SetLastPosFromToken($3.Pos)
        } |
        prefixexp '.' TReadonly {
            key := &ast.StringExpr{Value:"readonly"}
            key.SetPosFromToken($3.Pos)
            key.SetLastPosFromToken($3.Pos)
            $$ = &ast.AttrGetExpr{Object: $1, Key: key}
            $$.CopyPos($1)
            $$.SetLastPosFromToken($3.Pos)
        } |
        prefixexp '.' TAs {
            key := &ast.StringExpr{Value:"as"}
            key.SetPosFromToken($3.Pos)
            key.SetLastPosFromToken($3.Pos)
            $$ = &ast.AttrGetExpr{Object: $1, Key: key}
            $$.CopyPos($1)
            $$.SetLastPosFromToken($3.Pos)
        } |
        prefixexp '.' TAsserts {
            key := &ast.StringExpr{Value:"asserts"}
            key.SetPosFromToken($3.Pos)
            key.SetLastPosFromToken($3.Pos)
            $$ = &ast.AttrGetExpr{Object: $1, Key: key}
            $$.CopyPos($1)
            $$.SetLastPosFromToken($3.Pos)
        } |
        prefixexp '.' TIs {
            key := &ast.StringExpr{Value:"is"}
            key.SetPosFromToken($3.Pos)
            key.SetLastPosFromToken($3.Pos)
            $$ = &ast.AttrGetExpr{Object: $1, Key: key}
            $$.CopyPos($1)
            $$.SetLastPosFromToken($3.Pos)
        }

namelist:
        TIdent {
            $$ = []nameWithPos{{Name: $1.Str, Pos: $1.Pos}}
        } |
        namelist ','  TIdent {
            $$ = append($1, nameWithPos{Name: $3.Str, Pos: $3.Pos})
        }

exprlist:
        expr {
            $$ = []ast.Expr{$1}
        } |
        exprlist ',' expr {
            $$ = append($1, $3)
        }

expr:
        TNil {
            $$ = &ast.NilExpr{}
            $$.SetPosFromToken($1.Pos)
        } | 
        TFalse {
            $$ = &ast.FalseExpr{}
            $$.SetPosFromToken($1.Pos)
        } | 
        TTrue {
            $$ = &ast.TrueExpr{}
            $$.SetPosFromToken($1.Pos)
        } | 
        TNumber {
            $$ = &ast.NumberExpr{Value: $1.Str}
            $$.SetPosFromToken($1.Pos)
        } | 
        T3Comma {
            $$ = &ast.Comma3Expr{}
            $$.SetPosFromToken($1.Pos)
        } |
        function {
            $$ = $1
        } | 
        prefixexp {
            $$ = $1
        } |
        string {
            $$ = $1
        } |
        tableconstructor {
            $$ = $1
        } |
        expr TOr expr {
            $$ = &ast.LogicalOpExpr{Lhs: $1, Operator: "or", Rhs: $3}
            $$.CopyPos($1)
            $$.CopyLastPos($3)
        } |
        expr TAnd expr {
            $$ = &ast.LogicalOpExpr{Lhs: $1, Operator: "and", Rhs: $3}
            $$.CopyPos($1)
            $$.CopyLastPos($3)
        } |
        expr '>' expr {
            $$ = &ast.RelationalOpExpr{Lhs: $1, Operator: ">", Rhs: $3}
            $$.CopyPos($1)
            $$.CopyLastPos($3)
        } |
        expr '<' expr {
            $$ = &ast.RelationalOpExpr{Lhs: $1, Operator: "<", Rhs: $3}
            $$.CopyPos($1)
            $$.CopyLastPos($3)
        } |
        expr TGte expr {
            $$ = &ast.RelationalOpExpr{Lhs: $1, Operator: ">=", Rhs: $3}
            $$.CopyPos($1)
            $$.CopyLastPos($3)
        } |
        expr TLte expr {
            $$ = &ast.RelationalOpExpr{Lhs: $1, Operator: "<=", Rhs: $3}
            $$.CopyPos($1)
            $$.CopyLastPos($3)
        } |
        expr TEqeq expr {
            $$ = &ast.RelationalOpExpr{Lhs: $1, Operator: "==", Rhs: $3}
            $$.CopyPos($1)
            $$.CopyLastPos($3)
        } |
        expr TNeq expr {
            $$ = &ast.RelationalOpExpr{Lhs: $1, Operator: "~=", Rhs: $3}
            $$.CopyPos($1)
            $$.CopyLastPos($3)
        } |
        expr T2Comma expr {
            $$ = &ast.StringConcatOpExpr{Lhs: $1, Rhs: $3}
            $$.CopyPos($1)
            $$.CopyLastPos($3)
        } |
        expr '+' expr {
            $$ = &ast.ArithmeticOpExpr{Lhs: $1, Operator: "+", Rhs: $3}
            $$.CopyPos($1)
            $$.CopyLastPos($3)
        } |
        expr '-' expr {
            $$ = &ast.ArithmeticOpExpr{Lhs: $1, Operator: "-", Rhs: $3}
            $$.CopyPos($1)
            $$.CopyLastPos($3)
        } |
        expr '*' expr {
            $$ = &ast.ArithmeticOpExpr{Lhs: $1, Operator: "*", Rhs: $3}
            $$.CopyPos($1)
            $$.CopyLastPos($3)
        } |
        expr '/' expr {
            $$ = &ast.ArithmeticOpExpr{Lhs: $1, Operator: "/", Rhs: $3}
            $$.CopyPos($1)
            $$.CopyLastPos($3)
        } |
        expr '%' expr {
            $$ = &ast.ArithmeticOpExpr{Lhs: $1, Operator: "%", Rhs: $3}
            $$.CopyPos($1)
            $$.CopyLastPos($3)
        } |
        expr '^' expr {
            $$ = &ast.ArithmeticOpExpr{Lhs: $1, Operator: "^", Rhs: $3}
            $$.CopyPos($1)
            $$.CopyLastPos($3)
        } |
        expr '&' expr {
            $$ = &ast.ArithmeticOpExpr{Lhs: $1, Operator: "&", Rhs: $3}
            $$.CopyPos($1)
            $$.CopyLastPos($3)
        } |
        expr '|' expr {
            $$ = &ast.ArithmeticOpExpr{Lhs: $1, Operator: "|", Rhs: $3}
            $$.CopyPos($1)
            $$.CopyLastPos($3)
        } |
        expr '~' expr {
            $$ = &ast.ArithmeticOpExpr{Lhs: $1, Operator: "~", Rhs: $3}
            $$.CopyPos($1)
            $$.CopyLastPos($3)
        } |
        expr TShl expr {
            $$ = &ast.ArithmeticOpExpr{Lhs: $1, Operator: "<<", Rhs: $3}
            $$.CopyPos($1)
            $$.CopyLastPos($3)
        } |
        expr TShr expr {
            $$ = &ast.ArithmeticOpExpr{Lhs: $1, Operator: ">>", Rhs: $3}
            $$.CopyPos($1)
            $$.CopyLastPos($3)
        } |
        expr TIdiv expr {
            $$ = &ast.ArithmeticOpExpr{Lhs: $1, Operator: "//", Rhs: $3}
            $$.CopyPos($1)
            $$.CopyLastPos($3)
        } |
        '-' expr %prec UNARY {
            $$ = &ast.UnaryMinusOpExpr{Expr: $2}
            $$.CopyPos($2)
            $$.CopyLastPos($2)
        } |
        TNot expr %prec UNARY {
            $$ = &ast.UnaryNotOpExpr{Expr: $2}
            $$.SetPosFromToken($1.Pos)
            $$.CopyLastPos($2)
        } |
        '#' expr %prec UNARY {
            $$ = &ast.UnaryLenOpExpr{Expr: $2}
            $$.CopyPos($2)
            $$.CopyLastPos($2)
        } |
        '~' expr %prec UNARY {
            $$ = &ast.UnaryBNotOpExpr{Expr: $2}
            $$.CopyPos($2)
            $$.CopyLastPos($2)
        } |
        expr TAs typeexpr {
            $$ = &ast.CastExpr{Expr: $1, Type: $3}
            $$.CopyPos($1)
            $$.CopyLastPos($3)
        } |
        expr T2Colon typeexpr {
            $$ = &ast.CastExpr{Expr: $1, Type: $3}
            $$.CopyPos($1)
            $$.CopyLastPos($3)
        } |
        expr TBang {
            $$ = &ast.NonNilAssertExpr{Expr: $1}
            $$.CopyPos($1)
            $$.CopyLastPos($1)
        }

string: 
        TString {
            $$ = &ast.StringExpr{Value: $1.Str}
            $$.SetPosFromToken($1.Pos)
        } 

prefixexp:
        var {
            $$ = $1
        } |
        afunctioncall {
            $$ = $1
        } |
        functioncall {
            $$ = $1
        } |
        '(' expr ')' {
            if ex, ok := $2.(*ast.Comma3Expr); ok {
                ex.AdjustRet = true
            }
            $$ = $2
            $$.SetPosFromToken($1.Pos)
            $$.SetLastPosFromToken($3.Pos)
        }

afunctioncall:
        '(' functioncall ')' {
            $2.(*ast.FuncCallExpr).AdjustRet = true
            $$ = $2
        }

functioncall:
        prefixexp args {
            $$ = &ast.FuncCallExpr{Func: $1, Args: $2}
            $$.CopyPos($1)
            setLastPosFromExprs($$, $2, $1)
        } |
        prefixexp ':' TIdent args {
            $$ = &ast.FuncCallExpr{Method: $3.Str, Receiver: $1, Args: $4}
            $$.CopyPos($1)
            setLastPosFromExprs($$, $4, $1)
        } |
        prefixexp ':' TType args {
            $$ = &ast.FuncCallExpr{Method: "type", Receiver: $1, Args: $4}
            $$.CopyPos($1)
            setLastPosFromExprs($$, $4, $1)
        } |
        prefixexp ':' TInterface args {
            $$ = &ast.FuncCallExpr{Method: "interface", Receiver: $1, Args: $4}
            $$.CopyPos($1)
            setLastPosFromExprs($$, $4, $1)
        } |
        prefixexp ':' TReadonly args {
            $$ = &ast.FuncCallExpr{Method: "readonly", Receiver: $1, Args: $4}
            $$.CopyPos($1)
            setLastPosFromExprs($$, $4, $1)
        } |
        prefixexp ':' TAs args {
            $$ = &ast.FuncCallExpr{Method: "as", Receiver: $1, Args: $4}
            $$.CopyPos($1)
            setLastPosFromExprs($$, $4, $1)
        } |
        prefixexp ':' TAsserts args {
            $$ = &ast.FuncCallExpr{Method: "asserts", Receiver: $1, Args: $4}
            $$.CopyPos($1)
            setLastPosFromExprs($$, $4, $1)
        } |
        prefixexp ':' TIs args {
            $$ = &ast.FuncCallExpr{Method: "is", Receiver: $1, Args: $4}
            $$.CopyPos($1)
            setLastPosFromExprs($$, $4, $1)
        }

args:
        '(' ')' {
            if yylex.(*Lexer).PNewLine {
               yylex.(*Lexer).TokenError($1, "ambiguous syntax (function call x new statement)")
            }
            $$ = []ast.Expr{}
        } |
        '(' exprlist ')' {
            if yylex.(*Lexer).PNewLine {
               yylex.(*Lexer).TokenError($1, "ambiguous syntax (function call x new statement)")
            }
            $$ = $2
        } |
        tableconstructor {
            $$ = []ast.Expr{$1}
        } | 
        string {
            $$ = []ast.Expr{$1}
        }

function:
        TFunction funcbody {
            $$ = &ast.FunctionExpr{TypeParams: $2.TypeParams, ParList:$2.ParList, ReturnTypes: $2.ReturnTypes, Stmts: $2.Stmts}
            $$.SetPosFromToken($1.Pos)
            $$.CopyLastPos($2)
        }

funcbody:
        '(' parlist ')' returntypeannot block TEnd {
            $$ = &ast.FunctionExpr{ParList: $2, ReturnTypes: $4, Stmts: $5}
            $$.SetPosFromToken($1.Pos)
            $$.SetLastPosFromToken($6.Pos)
        } |
        '(' ')' returntypeannot block TEnd {
            $$ = &ast.FunctionExpr{ParList: &ast.ParList{HasVargs: false, Names: []string{}}, ReturnTypes: $3, Stmts: $4}
            $$.SetPosFromToken($1.Pos)
            $$.SetLastPosFromToken($5.Pos)
        } |
        typeparams '(' parlist ')' returntypeannot block TEnd {
            $$ = &ast.FunctionExpr{TypeParams: $1, ParList: $3, ReturnTypes: $5, Stmts: $6}
            $$.SetPosFromToken($2.Pos)
            $$.SetLastPosFromToken($7.Pos)
        } |
        typeparams '(' ')' returntypeannot block TEnd {
            $$ = &ast.FunctionExpr{TypeParams: $1, ParList: &ast.ParList{HasVargs: false, Names: []string{}}, ReturnTypes: $4, Stmts: $5}
            $$.SetPosFromToken($2.Pos)
            $$.SetLastPosFromToken($6.Pos)
        }

parlist:
        T3Comma {
            $$ = &ast.ParList{HasVargs: true, Names: []string{}}
        } |
        T3Comma ':' typeexpr {
            $$ = &ast.ParList{HasVargs: true, VarargType: $3, Names: []string{}}
        } |
        typednamelist {
            names, _, types := splitTypedNames($1)
            $$ = &ast.ParList{HasVargs: false, Names: names, Types: types}
        } |
        typednamelist ',' T3Comma {
            names, _, types := splitTypedNames($1)
            $$ = &ast.ParList{HasVargs: true, Names: names, Types: types}
        } |
        typednamelist ',' T3Comma ':' typeexpr {
            names, _, types := splitTypedNames($1)
            $$ = &ast.ParList{HasVargs: true, VarargType: $5, Names: names, Types: types}
        }


tableconstructor:
        '{' '}' {
            $$ = &ast.TableExpr{Fields: []*ast.Field{}}
            $$.SetPosFromToken($1.Pos)
            $$.SetLastPosFromToken($2.Pos)
        } |
        '{' fieldlist '}' {
            $$ = &ast.TableExpr{Fields: $2}
            $$.SetPosFromToken($1.Pos)
            $$.SetLastPosFromToken($3.Pos)
        }


fieldlist:
        field {
            $$ = []*ast.Field{$1}
        } | 
        fieldlist fieldsep field {
            $$ = append($1, $3)
        } | 
        fieldlist fieldsep {
            $$ = $1
        }

field:
        fieldname '=' expr {
            $$ = &ast.Field{Key: &ast.StringExpr{Value:$1}, Value: $3}
        } |
        '[' expr ']' '=' expr {
            $$ = &ast.Field{Key: $2, Value: $5}
        } |
        expr {
            $$ = &ast.Field{Value: $1}
        }

/* fieldname allows identifiers and contextual keywords as table field names */
fieldname:
        TIdent {
            $$ = $1.Str
        } |
        TType {
            $$ = "type"
        } |
        TInterface {
            $$ = "interface"
        } |
        TReadonly {
            $$ = "readonly"
        } |
        TAs {
            $$ = "as"
        } |
        TAsserts {
            $$ = "asserts"
        } |
        TIs {
            $$ = "is"
        } |
        TKeyof {
            $$ = "keyof"
        } |
        TExtends {
            $$ = "extends"
        }

typefieldname:
        TType {
            $$ = "type"
        } |
        TInterface {
            $$ = "interface"
        } |
        TReadonly {
            $$ = "readonly"
        } |
        TAs {
            $$ = "as"
        } |
        TAsserts {
            $$ = "asserts"
        } |
        TIs {
            $$ = "is"
        } |
        TKeyof {
            $$ = "keyof"
        } |
        TExtends {
            $$ = "extends"
        } |
        TTypeof {
            $$ = "typeof"
        }

fieldsep:
        ',' {
            $$ = ","
        } |
        ';' {
            $$ = ";"
        }

/* Type expressions */

returntypeannot:
        /* empty */ {
            $$ = nil
        } |
        ':' typeexprlist {
            $$ = $2
        } |
        ':' '(' ')' {
            $$ = nil
        } |
        ':' '(' typeexpr ',' typeexprlist ')' {
            $$ = append([]ast.TypeExpr{$3}, $5...)
        }

typeexpr:
        simpletypeexpr {
            $$ = $1
        } |
        typeexpr '|' simpletypeexpr {
            if union, ok := $1.(*ast.UnionTypeExpr); ok {
                union.Types = append(union.Types, $3)
                $$ = union
            } else {
                $$ = &ast.UnionTypeExpr{Types: []ast.TypeExpr{$1, $3}}
            }
        } |
        typeexpr '&' simpletypeexpr {
            if inter, ok := $1.(*ast.IntersectionTypeExpr); ok {
                inter.Types = append(inter.Types, $3)
                $$ = inter
            } else {
                $$ = &ast.IntersectionTypeExpr{Types: []ast.TypeExpr{$1, $3}}
            }
        } |
        simpletypeexpr TExtends simpletypeexpr TQuestion typeexpr ':' typeexpr {
            $$ = &ast.ConditionalTypeExpr{Check: $1, Extends: $3, Then: $5, Else: $7}
            $$.CopyPos($1)
        }

simpletypeexpr:
        primarytypeexpr {
            $$ = $1
        } |
        primarytypeexpr annotations {
            if prim, ok := $1.(*ast.PrimitiveTypeExpr); ok {
                prim.Annotations = $2
                $$ = prim
            } else if arr, ok := $1.(*ast.ArrayTypeExpr); ok {
                arr.ArrayAnnotations = $2
                $$ = arr
            } else {
                $$ = $1
            }
        } |
        simpletypeexpr TQuestion {
            $$ = &ast.OptionalTypeExpr{Inner: $1}
        }

primarytypeexpr:
        TNil {
            $$ = &ast.PrimitiveTypeExpr{Name: "nil"}
            $$.SetPosFromToken($1.Pos)
        } |
        TTrue {
            $$ = &ast.LiteralTypeExpr{Value: true}
            $$.SetPosFromToken($1.Pos)
        } |
        TFalse {
            $$ = &ast.LiteralTypeExpr{Value: false}
            $$.SetPosFromToken($1.Pos)
        } |
        TString {
            $$ = &ast.LiteralTypeExpr{Value: $1.Str}
            $$.SetPosFromToken($1.Pos)
        } |
        TNumber {
            num, _ := parseNumber($1.Str)
            $$ = &ast.LiteralTypeExpr{Value: num}
            $$.SetPosFromToken($1.Pos)
        } |
        TIdent {
            $$ = &ast.PrimitiveTypeExpr{Name: $1.Str}
            $$.SetPosFromToken($1.Pos)
        } |
        TIdent '.' TIdent {
            $$ = &ast.TypeRefExpr{Path: []string{$1.Str, $3.Str}}
            $$.SetPosFromToken($1.Pos)
        } |
        TIdent '<' typeexprlist closegt {
            $$ = &ast.GenericTypeExpr{
                Base: &ast.TypeRefExpr{Path: []string{$1.Str}},
                Args: $3,
            }
            $$.SetPosFromToken($1.Pos)
        } |
        '{' typeexpr '}' {
            $$ = &ast.ArrayTypeExpr{Element: $2}
            $$.SetPosFromToken($1.Pos)
        } |
        '{' '[' typeexpr ']' ':' typeexpr '}' {
            $$ = &ast.MapTypeExpr{Key: $3, Value: $6}
            $$.SetPosFromToken($1.Pos)
        } |
        '{' typefieldlist '}' {
            $$ = &ast.RecordTypeExpr{Fields: $2}
            $$.SetPosFromToken($1.Pos)
        } |
        '(' funcparamlist ')' TArrow '(' typeexprlist2 ')' {
            $$ = &ast.FunctionTypeExpr{Params: $2, Returns: $6}
            $$.SetPosFromToken($1.Pos)
        } |
        '(' funcparamlist ')' TArrow '(' ')' {
            $$ = &ast.FunctionTypeExpr{Params: $2, Returns: []ast.TypeExpr{}}
            $$.SetPosFromToken($1.Pos)
        } |
        '(' funcparamlist ')' TArrow typeexpr {
            $$ = &ast.FunctionTypeExpr{Params: $2, Returns: []ast.TypeExpr{$5}}
            $$.SetPosFromToken($1.Pos)
        } |
        '(' ')' TArrow '(' typeexprlist2 ')' {
            $$ = &ast.FunctionTypeExpr{Params: []ast.FunctionParamExpr{}, Returns: $5}
            $$.SetPosFromToken($1.Pos)
        } |
        '(' ')' TArrow typeexpr {
            $$ = &ast.FunctionTypeExpr{Params: []ast.FunctionParamExpr{}, Returns: []ast.TypeExpr{$4}}
            $$.SetPosFromToken($1.Pos)
        } |
        '(' ')' TArrow '(' ')' {
            $$ = &ast.FunctionTypeExpr{Params: []ast.FunctionParamExpr{}, Returns: []ast.TypeExpr{}}
            $$.SetPosFromToken($1.Pos)
        } |
        '(' '(' funcparamlist ')' TArrow typeexpr ')' {
            $$ = &ast.FunctionTypeExpr{Params: $3, Returns: []ast.TypeExpr{$6}}
            $$.SetPosFromToken($1.Pos)
        } |
        '(' '(' ')' TArrow typeexpr ')' {
            $$ = &ast.FunctionTypeExpr{Params: []ast.FunctionParamExpr{}, Returns: []ast.TypeExpr{$5}}
            $$.SetPosFromToken($1.Pos)
        } |
        '(' '(' funcparamlist ')' TArrow '(' typeexprlist2 ')' ')' {
            $$ = &ast.FunctionTypeExpr{Params: $3, Returns: $7}
            $$.SetPosFromToken($1.Pos)
        } |
        '(' '(' funcparamlist ')' TArrow '(' ')' ')' {
            $$ = &ast.FunctionTypeExpr{Params: $3, Returns: []ast.TypeExpr{}}
            $$.SetPosFromToken($1.Pos)
        } |
        '(' '(' ')' TArrow '(' typeexprlist2 ')' ')' {
            $$ = &ast.FunctionTypeExpr{Params: []ast.FunctionParamExpr{}, Returns: $6}
            $$.SetPosFromToken($1.Pos)
        } |
        '(' '(' ')' TArrow '(' ')' ')' {
            $$ = &ast.FunctionTypeExpr{Params: []ast.FunctionParamExpr{}, Returns: []ast.TypeExpr{}}
            $$.SetPosFromToken($1.Pos)
        } |
        TFun '(' funcparamlist ')' ':' typeexpr {
            $$ = &ast.FunctionTypeExpr{Params: $3, Returns: []ast.TypeExpr{$6}}
            $$.SetPosFromToken($1.Pos)
        } |
        TFun '(' funcparamlist ')' ':' '(' typeexprlist2 ')' {
            $$ = &ast.FunctionTypeExpr{Params: $3, Returns: $7}
            $$.SetPosFromToken($1.Pos)
        } |
        TFun '(' ')' ':' typeexpr {
            $$ = &ast.FunctionTypeExpr{Params: []ast.FunctionParamExpr{}, Returns: []ast.TypeExpr{$5}}
            $$.SetPosFromToken($1.Pos)
        } |
        TFun '(' ')' ':' '(' typeexprlist2 ')' {
            $$ = &ast.FunctionTypeExpr{Params: []ast.FunctionParamExpr{}, Returns: $6}
            $$.SetPosFromToken($1.Pos)
        } |
        TFun '(' funcparamlist ')' ':' '(' ')' {
            $$ = &ast.FunctionTypeExpr{Params: $3, Returns: []ast.TypeExpr{}}
            $$.SetPosFromToken($1.Pos)
        } |
        TFun '(' ')' ':' '(' ')' {
            $$ = &ast.FunctionTypeExpr{Params: []ast.FunctionParamExpr{}, Returns: []ast.TypeExpr{}}
            $$.SetPosFromToken($1.Pos)
        } |
        TFun '(' funcparamlist ')' {
            $$ = &ast.FunctionTypeExpr{Params: $3, Returns: nil}
            $$.SetPosFromToken($1.Pos)
        } |
        TFun '(' ')' {
            $$ = &ast.FunctionTypeExpr{Params: []ast.FunctionParamExpr{}, Returns: nil}
            $$.SetPosFromToken($1.Pos)
        } |
        TInterface '{' typefieldlist '}' {
            $$ = &ast.RecordTypeExpr{Fields: $3}
            $$.SetPosFromToken($1.Pos)
        } |
        TInterface '{' '}' {
            $$ = &ast.RecordTypeExpr{Fields: nil}
            $$.SetPosFromToken($1.Pos)
        } |
        primarytypeexpr '[' ']' {
            $$ = &ast.ArrayTypeExpr{Element: $1}
            $$.CopyPos($1)
        } |
        TReadonly '{' typeexpr '}' {
            arr := &ast.ArrayTypeExpr{Element: $3, Readonly: true}
            arr.SetPosFromToken($1.Pos)
            $$ = arr
        } |
        TReadonly '{' '[' typeexpr ']' ':' typeexpr '}' {
            m := &ast.MapTypeExpr{Key: $4, Value: $7, Readonly: true}
            m.SetPosFromToken($1.Pos)
            $$ = m
        } |
        TReadonly '{' typefieldlist '}' {
            $$ = &ast.RecordTypeExpr{Fields: $3, Readonly: true}
            $$.SetPosFromToken($1.Pos)
        } |
        '{' '}' {
            $$ = &ast.RecordTypeExpr{Fields: nil}
            $$.SetPosFromToken($1.Pos)
        } |
        TAsserts TIdent {
            $$ = &ast.AssertsTypeExpr{ParamName: $2.Str, NarrowTo: nil}
            $$.SetPosFromToken($1.Pos)
        } |
        TAsserts TIdent TIs typeexpr {
            $$ = &ast.AssertsTypeExpr{ParamName: $2.Str, NarrowTo: $4}
            $$.SetPosFromToken($1.Pos)
        } |
        TTypeof '(' expr ')' {
            $$ = &ast.TypeOfExpr{Expr: $3}
            $$.SetPosFromToken($1.Pos)
        } |
        TKeyof '(' typeexpr ')' {
            $$ = &ast.KeyOfExpr{Inner: $3}
            $$.SetPosFromToken($1.Pos)
        } |
        primarytypeexpr '[' typeexpr ']' {
            $$ = &ast.IndexAccessExpr{Object: $1, Index: $3}
            $$.CopyPos($1)
        }

typeexprlist:
        typeexpr {
            $$ = []ast.TypeExpr{$1}
        } |
        typeexprlist ',' typeexpr {
            $$ = append($1, $3)
        }

typeexprlist2:
        typeexpr ',' typeexpr {
            $$ = []ast.TypeExpr{$1, $3}
        } |
        typeexprlist2 ',' typeexpr {
            $$ = append($1, $3)
        }

closegt:
        '>' {
        } |
        TShr {
            yylex.(*Lexer).PendingGT = &ast.Token{
                Type: '>',
                Str:  ">",
                Pos:  ast.Position{Source: $1.Pos.Source, Line: $1.Pos.Line, Column: $1.Pos.Column + 1},
            }
        }

funcparam:
        TIdent ':' typeexpr {
            $$ = ast.FunctionParamExpr{Name: $1.Str, Type: $3}
        } |
        typeexpr {
            $$ = ast.FunctionParamExpr{Name: "", Type: $1}
        }

funcparamlist:
        funcparam {
            $$ = []ast.FunctionParamExpr{$1}
        } |
        funcparamlist ',' funcparam {
            $$ = append($1, $3)
        } |
        T3Comma typeexpr {
            $$ = []ast.FunctionParamExpr{{Name: "...", Type: $2}}
        } |
        funcparamlist ',' T3Comma typeexpr {
            $$ = append($1, ast.FunctionParamExpr{Name: "...", Type: $4})
        }

typefieldlist:
        typefield {
            $$ = []ast.RecordFieldExpr{$1}
        } |
        typefieldlist ',' typefield {
            $$ = append($1, $3)
        } |
        typefieldlist ',' {
            $$ = $1
        }

typefield:
        TIdent ':' typeexpr {
            $$ = ast.RecordFieldExpr{Name: $1.Str, Type: $3, Optional: false}
        } |
        TIdent ':' typeexpr annotations {
            $$ = ast.RecordFieldExpr{Name: $1.Str, Type: $3, Optional: false, Annotations: $4}
        } |
        TIdent TQuestionColon typeexpr {
            $$ = ast.RecordFieldExpr{Name: $1.Str, Type: $3, Optional: true}
        } |
        TIdent TQuestionColon typeexpr annotations {
            $$ = ast.RecordFieldExpr{Name: $1.Str, Type: $3, Optional: true, Annotations: $4}
        } |
        typefieldname ':' typeexpr {
            $$ = ast.RecordFieldExpr{Name: $1, Type: $3, Optional: false}
        } |
        typefieldname ':' typeexpr annotations {
            $$ = ast.RecordFieldExpr{Name: $1, Type: $3, Optional: false, Annotations: $4}
        } |
        typefieldname TQuestionColon typeexpr {
            $$ = ast.RecordFieldExpr{Name: $1, Type: $3, Optional: true}
        } |
        typefieldname TQuestionColon typeexpr annotations {
            $$ = ast.RecordFieldExpr{Name: $1, Type: $3, Optional: true, Annotations: $4}
        }

annotations:
        annotation {
            $$ = []ast.AnnotationExpr{$1}
        } |
        annotations annotation {
            $$ = append($1, $2)
        }

annotation:
        '@' TIdent {
            $$ = ast.AnnotationExpr{Name: $2.Str, Args: nil}
        } |
        '@' TIdent '(' ')' {
            $$ = ast.AnnotationExpr{Name: $2.Str, Args: nil}
        } |
        '@' TIdent '(' exprlist ')' {
            $$ = ast.AnnotationExpr{Name: $2.Str, Args: $4}
        }

typeparams:
        '<' typeparamlist '>' {
            $$ = $2
        }

typeparamlist:
        typeparam {
            $$ = []ast.TypeParamExpr{$1}
        } |
        typeparamlist ',' typeparam {
            $$ = append($1, $3)
        }

typeparam:
        TIdent {
            $$ = ast.TypeParamExpr{Name: $1.Str, Constraint: nil}
        } |
        TIdent ':' typeexpr {
            $$ = ast.TypeParamExpr{Name: $1.Str, Constraint: $3}
        }

typednamelist:
        typedname {
            $$ = []typedNameEntry{$1}
        } |
        typednamelist ',' typedname {
            $$ = append($1, $3)
        }

typedname:
        TIdent {
            $$ = typedNameEntry{Name: $1.Str, Pos: $1.Pos, Type: nil}
        } |
        TIdent ':' typeexpr {
            $$ = typedNameEntry{Name: $1.Str, Pos: $1.Pos, Type: $3}
        }

interfaceextends:
        /* empty */ {
            $$ = nil
        } |
        ':' TIdent {
            $$ = []*ast.TypeRefExpr{{Path: []string{$2.Str}}}
        } |
        interfaceextends ',' TIdent {
            $$ = append($1, &ast.TypeRefExpr{Path: []string{$3.Str}})
        }

interfacebody:
        /* empty */ {
            $$ = nil
        } |
        interfacebody interfacemethod {
            $$ = append($1, $2)
        }

interfacemethod:
        TFunction TIdent '(' ')' returntypeannot {
            $$ = ast.InterfaceMethodExpr{
                Name: $2.Str,
                Type: &ast.FunctionTypeExpr{Params: []ast.FunctionParamExpr{}, Returns: $5},
            }
        } |
        TFunction TIdent '(' typednamelist ')' returntypeannot {
            $$ = ast.InterfaceMethodExpr{
                Name: $2.Str,
                Type: &ast.FunctionTypeExpr{Params: toFuncParams($4), Returns: $6},
            }
        }

%%

// nameWithPos holds a name with its token position
type nameWithPos struct {
	Name string
	Pos  ast.Position
}

// splitNameList extracts names and positions from nameWithPos entries
func splitNameList(entries []nameWithPos) ([]string, []ast.Position) {
	names := make([]string, len(entries))
	positions := make([]ast.Position, len(entries))
	for i, e := range entries {
		names[i] = e.Name
		positions[i] = e.Pos
	}
	return names, positions
}

// typedNameEntry holds a name with optional type annotation
type typedNameEntry struct {
	Name string
	Pos  ast.Position
	Type ast.TypeExpr
}

// splitTypedNames extracts names, positions, and types from typed name entries
func splitTypedNames(entries []typedNameEntry) ([]string, []ast.Position, []ast.TypeExpr) {
	names := make([]string, len(entries))
	positions := make([]ast.Position, len(entries))
	var types []ast.TypeExpr
	hasTypes := false
	for i, e := range entries {
		names[i] = e.Name
		positions[i] = e.Pos
		if e.Type != nil {
			hasTypes = true
		}
	}
	if hasTypes {
		types = make([]ast.TypeExpr, len(entries))
		for i, e := range entries {
			types[i] = e.Type
		}
	}
	return names, positions, types
}

// toFuncParams converts typedNameEntry slice to FunctionParamExpr slice
func toFuncParams(entries []typedNameEntry) []ast.FunctionParamExpr {
	params := make([]ast.FunctionParamExpr, len(entries))
	for i, e := range entries {
		params[i] = ast.FunctionParamExpr{Name: e.Name, Type: e.Type}
	}
	return params
}

func TokenName(c int) string {
	if c >= TAnd && c-TAnd < len(yyToknames) {
		if yyToknames[c-TAnd] != "" {
			return yyToknames[c-TAnd]
		}
	}
    return string([]byte{byte(c)})
}
