(def hm (-> {}
    (assoc :a 1)
    (assoc :b 2)))

(prn hm)
(prn (get hm :a))
