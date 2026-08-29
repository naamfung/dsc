(def a 1)
(def b (fn [x] x))
(def c (fn [x]
    (if (= a 1)
        (prn a (b 2))
        (prn "hello"))))
(c 3)
(prn "end")
