(do
;; ENV

;;  An environment is an atom referencing a map where keys are strings
;;  instead of symbols.  The outer environment is the value associated
;;  with the normally invalid :outer key.

;;  Private helper for new-env.
(def bind-env (fn [env b e]
  (if (empty? b)
    env
    (let [b0 (first b)]
      (if (= '& b0)
        (assoc env (str (nth b 1)) e)
        (bind-env (assoc env (str b0) (first e)) (rest b) (rest e)))))))

(def new-env (fn [& args]
  (if (<= (count args) 1)
    (atom {:outer (first args)})
    (atom (apply bind-env {:outer (first args)} (rest args))))))

(def env-find (fn [env k]
  (env-find-str env (str k))))

;;  Private helper for env-find and env-get.
(def env-find-str (fn [env ks]
  (if env
    (let [data @env]
      (if (contains? data ks)
        env
        (env-find-str (get data :outer) ks))))))

(def env-get (fn [env k]
  (let [ks (str k)
         e (env-find-str env ks)]
    (if e
      (get @e ks)
      (throw (str "'" ks "' not found"))))))

(def env-set (fn [env k v]
  (do
    (swap! env assoc (str k) v)
    v)))

;; CORE

(def _macro? (fn [x]
  (if (map? x)
    (contains? x :__MAL_MACRO__)
    false)))

(def core_ns '[* + - / < <= = > >= apply assoc atom atom? concat conj
  cons contains? count deref dissoc empty? false? first fn? get
  hash-map keys keyword keyword? list list? map map? meta nil?
  nth number? pr-str println prn read-string readline reset! rest seq
  sequential? slurp str string? swap! symbol symbol? throw time-ms
  true? vals vec vector vector? with-meta])

;; EVAL extends this stack trace-atom when propagating exceptions.  If the
;; exception reaches the REPL loop, the full trace-atom is printed.
(def trace-atom (atom ""))

;; read
(def READ read-string)


;; eval

(def qq-loop (fn [elt acc]
  (if (if (list? elt) (= (first elt) 'splice-unquote)) ; 2nd 'if' means 'and'
    (list 'concat (nth elt 1) acc)
    (list 'cons (QUASIQUOTE elt) acc))))
(def qq-foldr (fn [xs]
  (if (empty? xs)
    ()
    (qq-loop (first xs) (qq-foldr (rest xs))))))
(def QUASIQUOTE (fn [ast]
  (cond
    (vector? ast)            (list 'vec (qq-foldr ast))
    (map? ast)               (list 'quote ast)
    (symbol? ast)            (list 'quote ast)
    (not (list? ast))        ast
    (= (first ast) 'unquote) (nth ast 1)
    "else"                   (qq-foldr ast))))

(def MACROEXPAND (fn [ast env]
  (let [a0 (if (list? ast) (first ast))
         e  (if (symbol? a0) (env-find env a0))
         m  (if e (env-get e a0))]
    (if (_macro? m)
      (MACROEXPAND (apply (get m :__MAL_MACRO__) (rest ast)) env)
      ast))))

(def eval-ast (fn [ast env]
  ;; (do (prn "eval-ast" ast "/" (keys @env)) )
  (cond
    (symbol? ast) (env-get env ast)
    (list? ast)   (map (fn [exp] (EVAL exp env)) ast)
    (vector? ast) (vec (map (fn [exp] (EVAL exp env)) ast))
    (map? ast)    (apply hash-map
                    (apply concat
                      (map (fn [k] [k (EVAL (get ast k) env)])
                           (keys ast))))
    "else"        ast)))

(def LET (fn [env binds form]
  (if (empty? binds)
    (EVAL form env)
    (do
      (env-set env (first binds) (EVAL (nth binds 1) env))
      (LET env (rest (rest binds)) form)))))

(def EVAL (fn [ast env]
  ;; (do (prn "EVAL" ast "/" (keys @env)) )
  (try
    (let [ast (MACROEXPAND ast env)]
      (if (not (list? ast))
        (eval-ast ast env)

        ;; apply list
        (let [a0 (first ast)]
          (cond
            (empty? ast)
            ast

            (= 'def a0)
            (env-set env (nth ast 1) (EVAL (nth ast 2) env))

            (= 'let a0)
            (LET (new-env env) (nth ast 1) (nth ast 2))

            (= 'quote a0)
            (nth ast 1)

            (= 'quasiquoteexpand a0)
            (QUASIQUOTE (nth ast 1))

            (= 'quasiquote a0)
            (EVAL (QUASIQUOTE (nth ast 1)) env)

            (= 'defmacro a0)
            (env-set env (nth ast 1) (hash-map :__MAL_MACRO__
                                               (EVAL (nth ast 2) env)))

            (= 'macroexpand a0)
            (MACROEXPAND (nth ast 1) env)

            (= 'try a0)
            (if (< (count ast) 3)
              (EVAL (nth ast 1) env)
                (try
                  (EVAL (nth ast 1) env)
                  (catch exc
                    (do
                      (reset! trace-atom "")
                        (let [a2 (nth ast 2)]
                          (EVAL (nth a2 2) (new-env env [(nth a2 1)] [exc])))))))

            (= 'do a0)
            (nth (eval-ast (rest ast) env) (- (count ast) 2))

            (= 'if a0)
            (if (EVAL (nth ast 1) env)
              (EVAL (nth ast 2) env)
              (if (> (count ast) 3)
                (EVAL (nth ast 3) env)))

            (= 'fn a0)
            (fn [& args] (EVAL (nth ast 2) (new-env env (nth ast 1) args)))

            "else"
            (let [el (eval-ast ast env)]
              (apply (first el) (rest el)))))))

    (catch exc
      (do
        (swap! trace-atom str "\n  in mal EVAL: " ast)
        (throw exc))))))

;; print
(def PRINT pr-str)

;; repl
(def repl-env (new-env))
(def rep (fn [strng]
  (PRINT (EVAL (READ strng) repl-env))))

;; core.mal: defined directly using mal
(map (fn [sym] (env-set repl-env sym (eval sym))) core_ns)
(env-set repl-env 'macro? _macro?)
(env-set repl-env 'eval (fn [ast] (EVAL ast repl-env)))
(env-set repl-env '*ARGV* (rest *ARGV*))

;; core.mal: defined using the new language itself
(rep (str "(def *host-language* \"" *host-language* "-mal\")"))
(rep "(def not (fn [a] (if a false true)))")
(rep ¬(def load-file (fn (f) (eval (read-string (str "(do " (slurp f) "\nnil)")))))¬)
(rep "(defmacro cond (fn (& xs) (if (> (count xs) 0) (list 'if (first xs) (if (> (count xs) 1) (nth xs 1) (throw \"odd number of forms to cond\")) (cons 'cond (rest (rest xs)))))))")

;; repl loop
(def repl-loop (fn [line]
  (if line
    (do
      (if (not (= "" line))
        (try
          (println (rep line))
          (catch exc
            (do
              (println "Uncaught exception:" exc @trace-atom)
              (reset! trace-atom "")))))
      (repl-loop (readline "mal-user> "))))))

;; main
  (if (empty? *ARGV*)
    (repl-loop "(println (str \"Mal [\" *host-language* \"]\"))")
    (rep (str "(load-file \"" (first *ARGV*) "\")"))))
