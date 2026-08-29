(do
    (def m (-> {}
        (assoc :b 2)
        (assoc :c true)
        (assoc :z 26)
        (assoc :a true)))
    (get m :a))
