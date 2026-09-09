;; $MODULE header-basic

(do
    (def *host-language* "go")

    (def not (fn [a]
        (if a
            false
            true)))

    (defmacro cond (fn [& xs]
        (if (> (count xs) 0)
            (list
                'if (first xs)
                    (if (> (count xs) 1)
                        (nth xs 1)
                        (throw "odd number of forms to cond"))
                    (cons 'cond (rest (rest xs)))))))

    (defmacro defn (fn [name params & body]
        `(def ~name
            (fn ~params ~@body)))))
