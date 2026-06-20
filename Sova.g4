grammar Sova;

file : fileHeader? stmt* EOF;

// --- File Header --- \\
fileHeader : packageDecl? sideDecl
           | sideDecl? packageDecl;

packageDecl : 'package' packagePath;
packagePath : pkgIdent ('/' pkgIdent)*;
pkgIdent : softId | SIDE_FRONTEND | SIDE_BACKEND | SIDE_SHARED | SIDE_SYNTH;

// softId admits both real identifiers AND the synth-only soft-reserved keywords (`where`, `to`, `append`) wherever an identifier is grammatically expected. The synth grammar's parser rules still expect these as literal tokens in their own positions (`emit append to ID`, `for ... where ...`) — the alternative-branch trick lets user code freely use the same words as function/field/method/variable names without colliding with the synth keywords. See Faithbook BUGS.md #20 for the motivation.
softId : ID | 'where' | 'to' | 'append' | 'tag';

sideDecl : 'on' side;
side : SIDE_FRONTEND
     | SIDE_SHARED
     | SIDE_BACKEND ('(' ID ')')?
     | SIDE_TEST
     | SIDE_SYNTH
     ;

// --- Statements --- \\
stmt : block
     | wireRulesetStmt
     | wireGroupStmt
     | varDeclStmt
     | exprStmt
     | ifStmt
     | switchStmt
     | breakStmt
     | continueStmt
     | returnStmt
     | guardStmt
     | forStmt
     | whileStmt
     | funcDeclStmt
     | externDecl
     | enumDeclStmt
     | typeDeclStmt
     | interfaceDeclStmt
     | mixinDeclStmt
     | typeAliasStmt
     | importStmt
     | testDeclStmt
     | groupDeclStmt
     | setupStmt
     | teardownStmt
     | assertStmt
     | asSessionStmt
     | goStmt
     | deferStmt
     | selectStmt
     | synthDeclStmt
     ;

goStmt : 'go' (exprStmt | block);
deferStmt : 'defer' (exprStmt | block);
selectStmt : 'select' LBRACE selectCase* selectDefaultCase? RBRACE;
selectCase : 'case' selectCaseGuard '=>' (stmt | block);
selectDefaultCase : 'default' '=>' (stmt | block);
selectCaseGuard : selectRecvBinding | expr;
selectRecvBinding : varDeclTarget (',' varDeclTarget)* '=' expr;

testDeclStmt : 'test' STRING_LITERAL (PARALLEL)? testTagList? block;
groupDeclStmt : 'group' STRING_LITERAL (PARALLEL)? testTagList? LBRACE groupItem* RBRACE;
testTagList : 'tag' ':' STRING_LITERAL (',' STRING_LITERAL)*;
asSessionStmt : 'asSession' ('(' STRING_LITERAL? ')')? block;
groupItem : testDeclStmt
          | groupDeclStmt
          | setupStmt
          | teardownStmt
          ;
setupStmt : ('setup' | 'setupAll') block;
teardownStmt : ('teardown' | 'teardownAll') block;
assertStmt : 'assert' expr;

wireGroupStmt : wireSpec LBRACE stmt* RBRACE;
wireRulesetStmt : 'wire' 'ruleset' ID wireOptions?;

importStmt : 'import' STRING_LITERAL ('using' usingClause)?;
usingClause : MULT
            | LBRACE ID (',' ID)* RBRACE
            ;

block : LBRACE stmt* RBRACE;

// Decl Statements
varDeclStmt : annotation* wireSpec? (LET|CONST) varDeclTarget (',' varDeclTarget)* '=' expr;
varDeclTarget : softId typeAnnot?
              | '_'
              ;

funcDeclStmt : annotation* wireSpec? 'func' softId genericParams? '(' funcParamList? ')' typeAnnot? sideDecl? block;
genericParams : LT genericParam (',' genericParam)* GT;
genericParam : ID (':' qualifiedRef ('+' qualifiedRef)*)? ('with' qualifiedRef ('+' qualifiedRef)*)?;
wireSpec : 'wire' (':' ID)? wireOptions?;
wireOptions : '(' wireOption (',' wireOption)* ')';
wireOption : ID ':' expr;
funcParamList : funcParam (',' funcParam)*;
funcParam : annotation* VARARG? softId typeAnnot ('=' expr)?;

externDecl : 'extern' 'default'? STRING_LITERAL? LBRACE externItem* RBRACE;
externItem : externFunc
           | externVar
           | typeDeclStmt
           | interfaceDeclStmt
           ;
externFunc : 'async'? 'func' softId genericParams? '(' funcParamList? ')' typeAnnot? '=' externMapping;
externVar : (LET|CONST) softId typeAnnot '=' externMapping;
externMapping : STRING_LITERAL                                              #SimpleExternMapping
              | LBRACE externSideMapping (',' externSideMapping)* RBRACE    #SharedExternMapping
              ;
externSideMapping : externSide ('(' STRING_LITERAL ')')? ':' STRING_LITERAL;
externSide : SIDE_FRONTEND
           | SIDE_BACKEND
           ;

// Enum Statements
enumDeclStmt : 'enum' ID enumPayloadDef? LBRACE enumBody RBRACE;
enumPayloadDef : '(' enumFieldDef (',' enumFieldDef)* ')';
enumFieldDef : ID typeAnnot ('=' expr)?;
enumBody : (enumCase (',' enumCase)*)? enumMethod*;
enumCase : ID enumCaseArgs? ('=' INT_LITERAL)?;
enumCaseArgs : '(' (expr (',' expr)*)? ')';
enumMethod : 'func' softId '(' funcParamList? ')' typeAnnot? block;

// Type Statements
typeDeclStmt : annotation* 'type' ID genericParams? typeClause* LBRACE typeMember* RBRACE;
typeClause : implementsClause | withClause ;
implementsClause : 'implements' ID (',' ID)*;
withClause : 'with' qualifiedRef (',' qualifiedRef)*;
qualifiedRef : softId ('.' softId)?;
typeMember : fieldDecl
           | ctorDecl
           | methodDecl
           | castDecl
           ;
fieldDecl : annotation* memberModifier* softId typeAnnot ('=' expr)?;
ctorDecl : annotation* memberModifier* 'new' '(' funcParamList? ')' block?;
methodDecl : annotation* memberModifier* 'func' methodName genericParams? '(' funcParamList? ')' typeAnnot? block?;
castDecl : annotation* memberModifier* 'cast' '(' ID typeAnnot ')' typeAnnot? block;
memberModifier : 'private' | 'shared';

annotation : '@' ID ('(' (annotationArg (',' annotationArg)*)? ')')?;
annotationArg : (softId ':')? expr;

composableChild : softId ('.' softId)? LBRACE composableChild* RBRACE   #ComposableBareChild
                | expr                                          #ComposableExprChild
                | ifStmt                                        #ComposableIfChild
                | forStmt                                       #ComposableForChild
                | whileStmt                                     #ComposableWhileChild
                | switchStmt                                    #ComposableSwitchChild
                ;
methodName : softId
           | 'op' opSymbol
           ;
opSymbol : PLUS | MINUS | MULT | DIV | MOD | EQUAL;

// Interface Statements
interfaceDeclStmt : 'interface' ID genericParams? LBRACE methodSignature* RBRACE;
methodSignature : memberModifier* 'func' softId '(' funcParamList? ')' typeAnnot?;

// Mixin Statements
mixinDeclStmt : 'mixin' ID LBRACE mixinMember* RBRACE;
mixinMember : fieldDecl | methodDecl ;

// Custom Annotations (Synth)
synthDeclStmt : SIDE_SYNTH ID synthParams? 'on' synthRequiredSide? synthTarget LBRACE synthBodyItem* RBRACE;
synthParams : '(' funcParamList? ')';
synthRequiredSide : SIDE_FRONTEND | SIDE_BACKEND | SIDE_SHARED;
synthTarget : synthTargetKind ID;
synthTargetKind : 'type' | 'func' | LET | ID;
synthBodyItem : synthEmitOn
              | synthEmitAppend
              | synthEmitField
              | synthEmitMethod
              | synthEmitCtor
              | synthForStmt
              ;
synthEmitOn : 'emit' 'on' ID LBRACE annotation* RBRACE;
synthEmitAppend : 'emit' 'append' 'to' ID LBRACE expr RBRACE;
synthEmitField : 'emit' fieldDecl;
synthEmitMethod : 'emit' methodDecl;
synthEmitCtor : 'emit' ctorDecl;
synthForStmt : 'for' ID 'in' synthIterable synthWhere? LBRACE synthBodyItem* RBRACE;
synthIterable : ID '.' ID;
synthWhere : 'where' synthBoolExpr;
synthBoolExpr : '!'? ID '.' ID;

// Type alias: transparent name for an existing type, optionally qualified.
typeAliasStmt : 'using' ID '=' type;

// Control Flow Statements
ifStmt : 'if' expr block elseIfBranch* elseBranch?;
elseIfBranch : 'else if' expr block;
elseBranch : 'else' block;

breakStmt : 'break' INT_LITERAL?;
continueStmt : 'continue' INT_LITERAL?;
returnStmt : 'return' (expr (',' expr)*)?;
guardStmt : 'guard' expr guardReturn?;
guardReturn : 'return' (expr (',' expr)*);

switchStmt : 'switch' expr LBRACE switchCase* defaultCase? RBRACE;
switchCase : 'case' expr (',' expr)* ':' stmt*;
defaultCase : 'default' ':' stmt*;

// loops
forStmt : 'for' forCondition? block;
forCondition : forIntCondition
             | forInCondition
             | forRangeCondition
             ;

forIntCondition : forIntConditionInit ';' expr ';' expr;
forIntConditionInit : ID typeAnnot? ('=' expr)?;
forInCondition : forInTarget (',' forInTarget)* 'in' expr; // e.g., for key, value in map, for item in collection, for item, index in collection or for key, value, index in map
forInTarget : ID | '_';
forRangeCondition : ID 'in' expr '..' expr;

whileStmt : 'while' expr block;

// --- Expressions --- \\
exprStmt : softId genericArgs '(' funcArgList? ')'              #GenericFuncCallExprStmt
         | expr '(' funcArgList? ')'                            #FuncCallExprStmt
         | (INC | DEC) softId                                   #PrefixUnaryExprStmt
         | softId (INC | DEC)                                   #PostfixUnaryExprStmt
         | softId ('.' softId)+ assignmentOp expr               #FieldAssignmentExprStmt
         | assignmentTarget (',' assignmentTarget)* '=' expr    #MultiAssignmentExprStmt
         | expr LBRACK expr RBRACK assignmentOp expr            #IndexAssignmentExprStmt
         | softId assignmentOp expr                             #AssignmentExprStmt
         ;

assignmentTarget : softId
                 | '_'
                 ;

expr : softId genericArgs '(' funcArgList? ')'                          #GenericFuncCallExpr
     | pkgIdent                                                         #IdExpr
     | expr '(' funcArgList? ')' LBRACE composableChild* RBRACE         #ComposableCallExpr
     | expr '::' LT typeAnnot (',' typeAnnot)* GT '(' funcArgList? ')'  #TurbofishCallExpr
     | expr '(' funcArgList? ')'                                        #FuncCallExpr
     | expr ('.' softId)+                                               #FieldAccessExpr
     | expr LBRACK expr? ':' expr? RBRACK                               #SliceRangeExpr
     | expr LBRACK expr RBRACK                                          #IndexExpr
     | unaryOp expr                                                     #UnaryExpr
     | (INC | DEC) expr                                                 #PrefixUnaryExpr
     | expr (INC | DEC)                                                 #PostfixUnaryExpr
     | expr '!'                                                         #OptionUnwrapExpr
     | expr 'as' '?'? typeAnnot                                         #AsExpr
     | expr 'instanceof' typeAnnot                                      #InstanceofExpr
     | expr '..' expr (LPAREN expr RPAREN)?                             #RangeExpr
     | expr (MULT | DIV | MOD) expr                                     #MulBinaryExpr
     | expr (PLUS | MINUS) expr                                         #AddBinaryExpr
     | expr (BIT_SHIFT_LEFT | BIT_SHIFT_RIGHT) expr                     #ShiftBinaryExpr
     | expr (LT | LE | GT | GE) expr                                    #CmpBinaryExpr
     | expr (EQUAL | NOT_EQUAL) expr                                    #EqBinaryExpr
     | expr BIT_AND expr                                                #BitAndBinaryExpr
     | expr BIT_XOR expr                                                #BitXorBinaryExpr
     | expr BIT_OR expr                                                 #BitOrBinaryExpr
     | expr AND expr                                                    #LAndBinaryExpr
     | expr OR expr                                                     #LOrBinaryExpr
     | expr COALESCE expr                                               #CoalesceExpr
     | LPAREN expr RPAREN                                               #GroupedExpr
     | expr '?' expr ':' expr                                           #TernaryExpr
     | literal                                                          #LitExpr
     | 'when' expr LBRACE whenCase* defaultWhenCase RBRACE              #WhenExpr
     | 'func' '(' funcParamList? ')' typeAnnot? block                   #FuncLiteralExpr
     | 'new' pkgIdent ('.' softId)? genericArgs? ('(' funcArgList? ')')?   #NewInstanceExpr
     | 'chan' '<' type '>' '(' expr? ')'                                #ChanInitExpr
     | '@'                                                              #SessionExpr
     ;

funcArgList : funcArg (',' funcArg)*;
funcArg : (softId ':')? expr;

whenCase : expr (',' expr)* '=>' expr;
defaultWhenCase : '_' '=>' expr;

unaryOp : PLUS
        | MINUS
        | BIT_NOT
        | NOT
        ;
assignmentOp : ASSIGN
             | ADD_ASSIGN
             | SUB_ASSIGN
             | MUL_ASSIGN
             | DIV_ASSIGN
             | MOD_ASSIGN
             | BIT_AND_ASSIGN
             | BIT_OR_ASSIGN
             | BIT_XOR_ASSIGN
             | BIT_SHIFT_LEFT_ASSIGN
             | BIT_SHIFT_RIGHT_ASSIGN
             ;

literal : INT_LITERAL
        | FLOAT_LITERAL
        | STRING_LITERAL
        | TEMPLATE_STRING
        | CHAR_LITERAL
        | (TRUE | FALSE)
        | NONE
        | array_literal
        | map_literal
        | tuple_literal
        ;
array_literal : '[' (expr (',' expr)* ','?)? ']';
map_literal : '{' (expr ':' expr (',' expr ':' expr)* ','?)? '}';
tuple_literal : '(' (expr (',' expr)* ','?)? ')';

// --- Types --- \\
typeAnnot : ':'? type; // make colon optional for better reading

type : primitiveType
     | optionType
     | sliceType
     | arrayType
     | mapType
     | tupleType
     | funcType
     | chanType
     | wildcardType
     | customType
     ;

chanType : 'chan' '<' type '>';

wildcardType : '?' (':' qualifiedRef ('+' qualifiedRef)*)? ('with' qualifiedRef ('+' qualifiedRef)*)?;

customType : pkgIdent ('.' ID)? genericArgs?;
genericArgs : LT type (',' type)* GT;

funcType : 'func' '(' funcTypeParamList? ')' typeAnnot? ;
funcTypeParamList : funcTypeParam (',' funcTypeParam)* ;
funcTypeParam : (ID ':')? type ;

primitiveType : INT | FLOAT | STRING | CHAR | BOOL | ANY | BYTE;

optionType : 'option' '<' type '>';

arrayType : '[' type INT_LITERAL ']';

sliceType : (LBRACK RBRACK)+ type;

mapType : 'map<' type ',' type '>';

tupleType : '(' tupleField (',' tupleField)* ')';
tupleField : (ID ':'?)? type;

// --- Lexer Rules --- \\
// Keywords
INT : 'int';
FLOAT : 'float';
STRING : 'string';
CHAR : 'char';
ANY : 'any';
BOOL : 'bool';
BYTE : 'byte';
TRUE : 'true';
FALSE : 'false';
NONE : 'none';

LET : 'let';
CONST : 'const';

SIDE_FRONTEND : 'frontend';
SIDE_BACKEND : 'backend';
SIDE_SHARED : 'shared';
SIDE_TEST : 'test';
SIDE_SYNTH : 'synth';
PARALLEL : 'parallel';

// Operators
VARARG : '...';

// Arithmetic Operators
PLUS : '+';
MINUS : '-';
MULT : '*';
DIV : '/';
MOD : '%';
INC : '++';
DEC : '--';

// Bitwise Operators
BIT_AND : '&';
BIT_OR : '|';
BIT_XOR : '^';
BIT_NOT : '~';
BIT_SHIFT_LEFT : '<<';
BIT_SHIFT_RIGHT : '>>';

// Comparison Operators
EQUAL : '==';
NOT_EQUAL : '!=';
LT : '<';
GT : '>';
LE : '<=';
GE : '>=';

// Logical Operators
AND : '&&';
OR : '||';
NOT : '!';

// Option Operators
COALESCE : '??';

// Grouping Operators
LPAREN : '(';
RPAREN : ')';
LBRACE : '{';
RBRACE : '}';
LBRACK : '[';
RBRACK : ']';

// Assignment Operators
ASSIGN : '=';
ADD_ASSIGN : '+=';
SUB_ASSIGN : '-=';
MUL_ASSIGN : '*=';
DIV_ASSIGN : '/=';
MOD_ASSIGN : '%=';
BIT_AND_ASSIGN : '&=';
BIT_OR_ASSIGN : '|=';
BIT_XOR_ASSIGN : '^=';
BIT_SHIFT_LEFT_ASSIGN : '<<=';
BIT_SHIFT_RIGHT_ASSIGN : '>>=';

// Literals
INT_LITERAL : [0-9]+ | '0x' [0-9a-fA-F]+ | '0b' [01]+;
FLOAT_LITERAL : [0-9]+ '.' [0-9]+;
STRING_LITERAL : '"' (~["\\] | '\\' .)* '"';
TEMPLATE_STRING : '`' (~[`\\] | '\\' .)* '`';
CHAR_LITERAL : '\'' . '\'';

ID : [a-zA-Z_][a-zA-Z0-9_]*;

// --- Skipped symbols --- \\
WS : [ \t\r\n]+ -> skip;
SEMI : ';' -> skip;
DOC_BLOCK_COMMENT : '/**' .*? '*/' -> channel(HIDDEN);
MULTI_COMMENT : '/*' .*? '*/' -> skip;
DOC_COMMENT : '///' ~[\r\n]* -> channel(HIDDEN);
COMMENT : '//' ~[\r\n]* -> skip;