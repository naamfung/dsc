(do
    (def a 1)

    (def b (fn [] (throw 9)))

    (b))