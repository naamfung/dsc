(do
    (def sum-with-sideeffects (fn [x y]
        ;; this area is executed only for side effects
        (- 1 1)
        ;; This is returns the value of the function
        (+ x y)))
    (assert (= 7 (sum-with-sideeffects 3 4)))
    true)