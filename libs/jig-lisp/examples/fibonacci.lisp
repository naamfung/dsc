(def fib
    (fn [n]
        (if (<= n 1)
            n
            (+
                (fib (- n 1))
                (fib (- n 2))))))
;; (prn (fib 1))
;; (prn (fib 2))
;; (prn (fib 3))
;; (prn (fib 4))
;; (prn (fib 5))
;; (prn (fib 6))
(def m 6)

(def s 6)

(prn (fib s))

(prn
    (fib
        (fib
            (fib m))))

(prn "Done")